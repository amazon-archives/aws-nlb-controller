/*
Copyright 2019 OTIE-SI.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nlb

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"

	awsHelper "github.com/awslabs/aws-nlb-controller/pkg/aws"
	"github.com/awslabs/aws-nlb-controller/pkg/finalizers"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"

	networkingv1alpha1 "github.com/awslabs/aws-nlb-controller/pkg/apis/networking/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new NLB Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNLB{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nlb-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to NLB
	err = c.Watch(&source.Kind{Type: &networkingv1alpha1.NLB{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by NLB - change this for objects you create
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &networkingv1alpha1.NLB{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNLB{}

// ReconcileNLB reconciles a NLB object
type ReconcileNLB struct {
	client.Client
	scheme *runtime.Scheme

	isTesting bool

	cfnSvc cloudformationiface.CloudFormationAPI
	ec2Svc ec2iface.EC2API
	asgSvc autoscalingiface.AutoScalingAPI
}

type cfnTemplateInput struct {
	NLBSpec    *networkingv1alpha1.NLBSpec
	VpcID      string
	TargetPort int
	SubnetIDs  []string
}

// Status codes and Finalizer Strings
var (
	StatusCFNCreateComplete = "StackCreationComplete"
	StatusCreateComplete    = "Complete"
	StatusCreating          = "Creating"
	StatusFailed            = "Failed"

	FinalizerCFNStack      = "cfn-stack.nlb.networking.amazonaws.com"
	FinalizerAttachTGtoASG = "tg-asg.nlb.networking.amazonaws.com"
)

// Reconcile reads that state of the cluster for a NLB object and makes changes based on the state read
// and what is in the NLB.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.amazonaws.com,resources=nlbs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.amazonaws.com,resources=nlbs/status,verbs=get;update;patch
func (r *ReconcileNLB) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the NLB instance
	instance := &networkingv1alpha1.NLB{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	var cfnSvc cloudformationiface.CloudFormationAPI
	var ec2Svc ec2iface.EC2API
	var asgSvc autoscalingiface.AutoScalingAPI

	if r.isTesting {
		cfnSvc = r.cfnSvc
		ec2Svc = r.ec2Svc
		asgSvc = r.asgSvc
	} else {
		session := session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
		}))
		cfnSvc = cloudformation.New(session)
		ec2Svc = ec2.New(session)
		asgSvc = autoscaling.New(session)
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if finalizers.HasFinalizer(instance, FinalizerAttachTGtoASG) {
			stack, err := awsHelper.DescribeStack(cfnSvc, getStackName(instance))
			if err != nil {
				log.Error(err, "error describing stack", "stackName", getStackName(instance), "instance", instance.GetName())
				return reconcile.Result{}, err
			}
			targetGroupARN := awsHelper.GetStackOutputValueforKey(stack, "TargetGroupARN")
			if err := r.detachASGfromLoadBalancerTG(ec2Svc, asgSvc, targetGroupARN); err != nil {
				log.Error(err, "error removing the TG from ASG, requeing..", "instance", instance.GetName())
				return reconcile.Result{}, err
			}
			log.Info("successfully removed the TG from ASG", "instance", instance.GetName())
			instance.SetFinalizers(finalizers.RemoveFinalizer(instance, FinalizerAttachTGtoASG))
			return reconcile.Result{Requeue: true}, r.Update(context.TODO(), instance)
		} else if finalizers.HasFinalizer(instance, FinalizerCFNStack) {
			log.Info("deletion timestamp found, deleting..", "instance", instance.GetName())
			stack, err := awsHelper.DescribeStack(cfnSvc, getStackName(instance))
			if err != nil && awsHelper.StackDoesNotExist(err) {
				log.Info("cfnstack not found, removing cfn finalizer", "instance", instance.GetName())
				instance.SetFinalizers(finalizers.RemoveFinalizer(instance, FinalizerCFNStack))
				return reconcile.Result{}, r.Update(context.TODO(), instance)
			}
			if err != nil {
				log.Error(err, "error describing stack while deleting the cfn", "stackName", getStackName(instance), "instance", instance.GetName())
				instance.Status.Status = StatusFailed
				r.Update(context.TODO(), instance)
				return reconcile.Result{}, err
			}
			if *stack.StackStatus == cloudformation.StackStatusDeleteComplete {
				log.Info("cfn stack deleted successfully, removing finalizers")
				instance.SetFinalizers(finalizers.RemoveFinalizer(instance, FinalizerCFNStack))
				return reconcile.Result{}, r.Update(context.TODO(), instance)
			}

			if *stack.StackStatus == cloudformation.StackStatusDeleteInProgress {
				return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
			}

			_, err = cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
				StackName: aws.String(getStackName(instance)),
			})
			if err != nil {
				log.Error(err, "error deleting the cfn stack", "stackName", getStackName(instance), "instance", instance.GetName())
				instance.Status.Status = StatusFailed
				return reconcile.Result{}, r.Update(context.TODO(), instance)
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}

	stack, err := awsHelper.DescribeStack(cfnSvc, getStackName(instance))
	if err != nil && awsHelper.StackDoesNotExist(err) {
		log.Info("no existing stack found, creating it", "stackName", getStackName(instance))
		if err = r.createNLB(ec2Svc, cfnSvc, instance); err != nil {
			log.Error(err, "error creating the cfn stack", "stackName", getStackName(instance), "instance", instance.GetName())
			instance.Status.Status = StatusFailed
			r.Update(context.TODO(), instance)
			return reconcile.Result{}, err
		}
		log.Info("stack creation initiated..", "stackName", getStackName(instance), "instance", instance.GetName())
		instance.Status.Status = StatusCreating
		instance.SetFinalizers(finalizers.AddFinalizer(instance, FinalizerCFNStack))
		r.Update(context.TODO(), instance)
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "error describing stack while creating the cfn", "stackName", getStackName(instance), "instance", instance.GetName())
		return reconcile.Result{}, err
	}

	if awsHelper.IsFailed(*stack.StackStatus) {
		log.Error(fmt.Errorf("cfn stack operation failed"), "cfn stack operation failed", "stackName", getStackName(instance), "status", *stack.StackStatus)
		instance.Status.Status = StatusFailed
		return reconcile.Result{}, r.Update(context.TODO(), instance)
	}

	if awsHelper.IsPending(*stack.StackStatus) {
		log.Info("stack creation not complete, requeueing..", "stackName", getStackName(instance), "status", *stack.StackStatus)
		return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if !awsHelper.IsComplete(*stack.StackStatus) {
		log.Error(fmt.Errorf("unknown cfn status encountered, failing"), "unknown cfn status encountered, failing", "stackName", getStackName(instance), "status", *stack.StackStatus)
		instance.Status.Status = StatusFailed
		return reconcile.Result{}, r.Update(context.TODO(), instance)
	}

	if !finalizers.HasFinalizer(instance, FinalizerAttachTGtoASG) {
		targetGroupARN := awsHelper.GetStackOutputValueforKey(stack, "TargetGroupARN")
		if err = r.attachASGtoLoadBalancerTG(ec2Svc, asgSvc, targetGroupARN); err != nil {
			return reconcile.Result{}, err
		}
		log.Info("successfully attached the ASG to the NLB's target group", "instance", instance.GetName())
		instance.Status.Status = StatusCreateComplete
		instance.SetFinalizers(finalizers.AddFinalizer(instance, FinalizerAttachTGtoASG))
		r.Update(context.TODO(), instance)
		return reconcile.Result{Requeue: true}, nil
	}

	log.Info("found the cfn stack", "stackName", getStackName(instance), "status", *stack.StackStatus, "instance", instance.GetName())
	loadBalancerDNSName := awsHelper.GetStackOutputValueforKey(stack, "LoadBalancerDNSName")
	if loadBalancerDNSName == nil {
		err = fmt.Errorf("error retrieving the loadbalancers DNS name")
		log.Error(err, err.Error(), "stackName", getStackName(instance))
		instance.Status.Status = StatusFailed
		return reconcile.Result{}, r.Update(context.TODO(), instance)
	}
	instance.Status.LoadBalancerDNSName = *loadBalancerDNSName
	instance.Status.Status = StatusCreateComplete
	r.Update(context.TODO(), instance)

	return reconcile.Result{}, nil
}

func getStackName(instance *networkingv1alpha1.NLB) string {
	return fmt.Sprintf("awsnlbctl-%s-%s", instance.Namespace, instance.Name)
}

func (r *ReconcileNLB) getEC2ReservationInfoFromNodes(ec2Client ec2iface.EC2API) ([]*ec2.Reservation, error) {
	nodeInstanceIds, err := r.getNodeInstanceIDs()
	if err != nil {
		log.Error(err, "unable to retrieve the K8s node object")
		return nil, err
	}
	reservations, err := awsHelper.GetReservationsforNodeInstanceIds(ec2Client, nodeInstanceIds)
	if err != nil {
		log.Error(err, "error retrieving the reservation details")
		return nil, err
	}

	return reservations, nil
}

func (r *ReconcileNLB) getNodeInstanceIDs() ([]string, error) {
	nodes := &corev1.NodeList{}

	err := r.List(context.TODO(), &client.ListOptions{}, nodes)
	if err != nil {
		return nil, err
	}

	var nodeInstanceIDs []string
	for _, item := range nodes.Items {
		if item.Spec.ProviderID != "" {
			r := regexp.MustCompile("i-[a-f0-9]+$")
			instanceID := r.FindString(item.Spec.ProviderID)
			if instanceID != "" {
				nodeInstanceIDs = append(nodeInstanceIDs, instanceID)
			}
		}
	}

	return nodeInstanceIDs, nil
}

func (r *ReconcileNLB) createNLB(ec2Svc ec2iface.EC2API, cfnSvc cloudformationiface.CloudFormationAPI, instance *networkingv1alpha1.NLB) error {
	reservations, err := r.getEC2ReservationInfoFromNodes(ec2Svc)
	if err != nil {
		log.Error(err, "error getting info from the K8s node", "instance", instance.GetName())
		return err
	}

	err = createNLBStack(cfnSvc, getStackName(instance), awsHelper.GetVPCIdFromReservations(reservations), awsHelper.GetSubnetIDsFromReservations(reservations), instance)

	return err
}

func createNLBStack(cfnSvc cloudformationiface.CloudFormationAPI, stackName string, vpcID string, subnetIDs []string, instance *networkingv1alpha1.NLB) error {
	templatizedCFN, err := awsHelper.GetCFNTemplateBody(cfnTemplate, cfnTemplateInput{
		NLBSpec:   &instance.Spec,
		VpcID:     vpcID,
		SubnetIDs: subnetIDs,
	})
	if err != nil {
		return err
	}
	_, err = cfnSvc.CreateStack(&cloudformation.CreateStackInput{
		TemplateBody: aws.String(templatizedCFN),
		StackName:    aws.String(stackName),
		Capabilities: []*string{aws.String("CAPABILITY_NAMED_IAM"), aws.String("CAPABILITY_IAM")},
		Tags:         []*cloudformation.Tag{},
	})
	return err
}

func (r *ReconcileNLB) attachASGtoLoadBalancerTG(ec2Client ec2iface.EC2API, asgClient autoscalingiface.AutoScalingAPI, targetGroupARN *string) error {
	reservations, err := r.getEC2ReservationInfoFromNodes(ec2Client)
	if err != nil {
		log.Error(err, "error getting info from the K8s node")
		return err
	}

	asgNames := awsHelper.GetASGsFromReservations(reservations)

	for _, asgName := range asgNames {
		_, err = asgClient.AttachLoadBalancerTargetGroups(&autoscaling.AttachLoadBalancerTargetGroupsInput{
			AutoScalingGroupName: &asgName,
			TargetGroupARNs:      []*string{targetGroupARN},
		})
		if err != nil {
			log.Error(err, "error attaching asg to TG", "asgName", asgName, "targetGroupARN", targetGroupARN)
			return err
		}
	}
	return nil
}

func (r *ReconcileNLB) detachASGfromLoadBalancerTG(ec2Client ec2iface.EC2API, asgClient autoscalingiface.AutoScalingAPI, targetGroupARN *string) error {
	reservations, err := r.getEC2ReservationInfoFromNodes(ec2Client)
	if err != nil {
		log.Error(err, "error getting info from the K8s node")
		return err
	}

	asgNames := awsHelper.GetASGsFromReservations(reservations)

	for _, asgName := range asgNames {
		_, err = asgClient.DetachLoadBalancerTargetGroups(&autoscaling.DetachLoadBalancerTargetGroupsInput{
			AutoScalingGroupName: &asgName,
			TargetGroupARNs:      []*string{targetGroupARN},
		})
		if err != nil {
			log.Error(err, "error detaching asg from TG", "asgName", asgName, "targetGroupARN", targetGroupARN)
			return err
		}
	}
	return nil
}

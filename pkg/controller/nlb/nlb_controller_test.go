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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudformation"

	networkingv1alpha1 "github.com/awslabs/aws-elb-controller/pkg/apis/networking/v1alpha1"
	awsHelper "github.com/awslabs/aws-elb-controller/pkg/aws"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-nlb", Namespace: "default"}}
var nlbKey = types.NamespacedName{Name: "foo-nlb", Namespace: "default"}

const timeout = time.Second * 5

func (reconcile *ReconcileNLB) getNodeInstanceIds() ([]string, error) {
	return []string{"i-foo", "i-bar"}, nil
}
func TestReconcileCreateAndDeleteWorks(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &networkingv1alpha1.NLB{
		ObjectMeta: metav1.ObjectMeta{Name: "foo-nlb", Namespace: "default"},
		Spec: networkingv1alpha1.NLBSpec{
			NodePort: 10000,
			Listeners: []networkingv1alpha1.Listener{
				networkingv1alpha1.Listener{
					Port:     100,
					Protocol: "UDP",
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	errDoesNotExist := awserr.New("ValidationError", `ValidationError: Stack with id nlb-blabla-test does not exist, status code: 400, request id: 8f05552d-3957`, nil)
	reconcile := &ReconcileNLB{
		Client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		isTesting: true,
		cfnSvc:    &awsHelper.MockCloudformationAPI{FailDescribe: true, Err: errDoesNotExist, Status: cloudformation.StackStatusCreateComplete},
		ec2Svc:    &awsHelper.MockEC2API{},
		asgSvc:    &awsHelper.MockAutoScalingAPI{},
	}

	recFn, requests := SetupTestReconcile(reconcile)
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-foo",
			Namespace: "default",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "i-123foobar",
		},
	}

	err = c.Create(context.TODO(), node)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Create the NLB object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	getNLB := &networkingv1alpha1.NLB{}
	g.Eventually(func() (string, error) {
		err := c.Get(context.TODO(), nlbKey, getNLB)
		return getNLB.Status.Status, err
	}).Should(gomega.Equal(StatusCreating))

	reconcile.cfnSvc = &awsHelper.MockCloudformationAPI{Status: cloudformation.StackStatusCreateComplete}
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	g.Eventually(func() (string, error) {
		err := c.Get(context.TODO(), nlbKey, getNLB)
		return getNLB.Status.Status, err
	}).Should(gomega.Equal(StatusCreateComplete))

	err = c.Delete(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}

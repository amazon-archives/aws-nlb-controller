# aws-nlb-controller
 The aws-nlb-controller creates and manages AWS Network Loadbalancer's as Kubernetes Custom Resources.
 This controller is built using the [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) framework. For more information [read their docs](https://book.kubebuilder.io/)

 ## Prerequisites
The controller uses the IAM Role of the eks nodes for creating the loadbalancers, so please ensure it has the appropriate permissions.

## Creating NLB's
1. To create a nlb, first install the controller onto your cluster. 
```sh
# Create ECR Repository
aws ecr create-repository --repository-name aws-nlb-controller
export REPOSITORY=`aws ecr describe-repositories --repository-name aws-nlb-controller | jq -r '.repositories[0].repositoryUri'`

# Build/tag the docker image
IMG=${REPOSITORY}:latest make docker-build

# Push the docker image
aws ecr get-login --no-include-email | bash -
docker push ${REPOSITORY}:latest

# Install the CRD's and deploy the controller
make deploy
```
2. Create a custom resource defining the parameters for the AWS NLB.
```sh
# Sample resource
$ cat config/samples/networking_v1alpha1_nlb.yaml 
apiVersion: networking.amazonaws.com/v1alpha1
kind: NLB
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: nlb-sample
spec:
  nodeport: 32212 # Port for forwarding the requests to.
  protocol: TCP # Protocol used for forwarding the requests, currently only TCP is supported
  listeners: 
    - port: 518 # Port to recieve requests on the loadbalancer
      protocol: TCP # Protocol used for recieving, currently only TCP is supported.
    - port: 519
      protocol: TCP

# Create the resource
$ kubectl apply -f config/samples/networking_v1alpha1_nlb.yaml 
```
3. Retrieve the Loadbalancers DNS Name from the status.
```sh
# Make sure the instance.Status.Status is 'Complete'
kubectl get nlbs.networking.amazonaws.com nlb-sample -o json | jq -r '.status.status'

# Get the Loadbalancers DNS Name
kubectl get nlbs.networking.amazonaws.com nlb-sample -o json | jq -r '.status.loadbalancerDNSname'
```

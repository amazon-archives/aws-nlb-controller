package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

// MockCloudformationAPI provides mocked interface to AWS Cloudformation service
type MockCloudformationAPI struct {
	cloudformationiface.CloudFormationAPI

	Err    error
	Status string

	FailDescribe bool
}

func (m *MockCloudformationAPI) DescribeStacks(input *cloudformation.DescribeStacksInput) (*cloudformation.DescribeStacksOutput, error) {
	if m.FailDescribe {
		return nil, m.Err
	}

	return &cloudformation.DescribeStacksOutput{
		Stacks: []*cloudformation.Stack{
			{
				StackName:   aws.String("foo"),
				StackStatus: aws.String(m.Status),
			},
		},
	}, nil
}

func (m *MockCloudformationAPI) CreateStack(*cloudformation.CreateStackInput) (*cloudformation.CreateStackOutput, error) {
	return &cloudformation.CreateStackOutput{}, nil
}

type MockEC2API struct {
	ec2iface.EC2API
}

func (e *MockEC2API) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			&ec2.Reservation{
				Instances: []*ec2.Instance{
					&ec2.Instance{
						SubnetId: aws.String("mysubnetID"),
						VpcId:    aws.String("vpcid-12345"),
						Tags: []*ec2.Tag{
							&ec2.Tag{
								Key:   aws.String("aws:autoscaling:groupName"),
								Value: aws.String("Foopla-1234"),
							},
						},
					},
				},
			},
		},
	}, nil
}

type MockAutoScalingAPI struct {
	autoscalingiface.AutoScalingAPI
}

func (a *MockAutoScalingAPI) AttachLoadBalancerTargetGroups(*autoscaling.AttachLoadBalancerTargetGroupsInput) (*autoscaling.AttachLoadBalancerTargetGroupsOutput, error) {
	return &autoscaling.AttachLoadBalancerTargetGroupsOutput{}, nil
}

func (a *MockAutoScalingAPI) DetachLoadBalancerTargetGroups(*autoscaling.DetachLoadBalancerTargetGroupsInput) (*autoscaling.DetachLoadBalancerTargetGroupsOutput, error) {
	return &autoscaling.DetachLoadBalancerTargetGroupsOutput{}, nil
}

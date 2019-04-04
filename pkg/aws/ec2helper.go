package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

//GetReservationsforNodeInstanceIds takes a list of node InstanceID's as input and returns their Reservation Info
func GetReservationsforNodeInstanceIds(ec2Client ec2iface.EC2API, nodeInstanceIds []string) ([]*ec2.Reservation, error) {
	output, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(nodeInstanceIds),
	})
	if err != nil {
		return nil, err
	}
	return output.Reservations, nil
}

// GetSubnetIDsFromReservations gets the list of unique subnetIDs from a given list of reservation info
func GetSubnetIDsFromReservations(reservations []*ec2.Reservation) []string {
	uniqueMap := map[string]bool{}
	subnetIds := []string{}
	for _, reservation := range reservations {
		for _, instance := range reservation.Instances {
			if *instance.SubnetId != "" {
				if _, ok := uniqueMap[*instance.SubnetId]; !ok {
					uniqueMap[*instance.SubnetId] = true
					subnetIds = append(subnetIds, *instance.SubnetId)
				}
			}
		}
	}
	return subnetIds
}

// GetVPCIdFromReservations returns the VPCID of the first instance found
func GetVPCIdFromReservations(reservations []*ec2.Reservation) string {
	return *reservations[0].Instances[0].VpcId
}

// GetASGsFromReservations returns the list of unique ASG's from a given list of reservation info
func GetASGsFromReservations(reservations []*ec2.Reservation) []string {
	uniqueMap := map[string]bool{}
	asgName := []string{}
	for _, reservation := range reservations {
		for _, instance := range reservation.Instances {
			for _, tag := range instance.Tags {
				if *tag.Key == "aws:autoscaling:groupName" {
					if _, ok := uniqueMap[*tag.Value]; !ok {
						uniqueMap[*tag.Value] = true
						asgName = append(asgName, *tag.Value)
					}
				}
			}
		}
	}
	return asgName
}

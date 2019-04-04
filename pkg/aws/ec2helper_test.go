package aws

import (
	"reflect"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func TestGetSubnetIDsFromReservations(t *testing.T) {
	tests := []struct {
		name         string
		reservations []*ec2.Reservation
		expected     []string
	}{
		{
			name: "Returns a single SubnetID",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{
						&ec2.Instance{
							SubnetId: aws.String("mysubnetID"),
						},
					},
				},
			},
			expected: []string{"mysubnetID"},
		},
		{
			name: "Omits empty subnetID's",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{
						&ec2.Instance{
							SubnetId: aws.String("mysubnetID"),
						},
					},
				},
				&ec2.Reservation{
					Instances: []*ec2.Instance{
						&ec2.Instance{
							SubnetId: aws.String(""),
						},
					},
				},
			},
			expected: []string{"mysubnetID"},
		},
		{
			name: "Returns only unique subnetIDs",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{
						&ec2.Instance{
							SubnetId: aws.String("mysubnetID"),
						},
						&ec2.Instance{
							SubnetId: aws.String("mysubnetID2"),
						},
						&ec2.Instance{
							SubnetId: aws.String("mysubnetID"),
						},
					},
				},
			},
			expected: []string{"mysubnetID", "mysubnetID2"},
		},
		{
			name: "Returns empty list if SubnetID is not found",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{},
				},
			},
			expected: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetSubnetIDsFromReservations(tc.reservations)
			if !checkStringArrayEquality(tc.expected, result) {
				t.Errorf("Expected: \n%+v\n\n Got:\n%+v", tc.expected, result)
			}
		})
	}
}

func TestGetASGsFromReservations(t *testing.T) {
	tests := []struct {
		name         string
		reservations []*ec2.Reservation
		expected     []string
	}{
		{
			name: "Returns a single ASG",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{
						&ec2.Instance{
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
			expected: []string{"Foopla-1234"},
		},
		{
			name: "Returns only unique ASGs",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{
						&ec2.Instance{
							Tags: []*ec2.Tag{
								&ec2.Tag{
									Key:   aws.String("aws:autoscaling:groupName"),
									Value: aws.String("Foopla-1234"),
								},
							},
						},
						&ec2.Instance{
							Tags: []*ec2.Tag{
								&ec2.Tag{
									Key:   aws.String("aws:autoscaling:groupName"),
									Value: aws.String("Foopla-1234"),
								},
							},
						},
						&ec2.Instance{
							Tags: []*ec2.Tag{
								&ec2.Tag{
									Key:   aws.String("aws:autoscaling:groupName"),
									Value: aws.String("Boopla-1234"),
								},
							},
						},
					},
				},
			},
			expected: []string{"Foopla-1234", "Boopla-1234"},
		},
		{
			name: "Returns empty list if ASG is not found",
			reservations: []*ec2.Reservation{
				&ec2.Reservation{
					Instances: []*ec2.Instance{},
				},
			},
			expected: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetASGsFromReservations(tc.reservations)
			if !checkStringArrayEquality(tc.expected, result) {
				t.Errorf("Expected: \n%+v\n\n Got:\n%+v", tc.expected, result)
			}
		})
	}
}

func checkStringArrayEquality(a, b []string) bool {
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

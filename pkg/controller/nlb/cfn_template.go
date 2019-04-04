package nlb

var cfnTemplate = `
---
AWSTemplateFormatVersion: '2010-09-09'
Resources:
  ExocompNLB:
    Type: "AWS::ElasticLoadBalancingV2::LoadBalancer"
    Properties:
      Scheme: "internal"
      Subnets:
{{- range $i, $e := .SubnetIDs }}
      - {{ $e }}
{{- end }}
      Type: "network"
  ExocompTG:
    Type: "AWS::ElasticLoadBalancingV2::TargetGroup"
    Properties:
      Port: {{ .NLBSpec.NodePort }}
      Protocol: {{ .NLBSpec.Protocol }}
      VpcId: {{ .VpcID }}
{{- range $i, $e := .NLBSpec.Listeners }}
  ExocompListener{{ $i }}:
    Type: "AWS::ElasticLoadBalancingV2::Listener"
    Properties:
      DefaultActions:
      - Type: "forward"
        TargetGroupArn: !Ref ExocompTG
      LoadBalancerArn: !Ref ExocompNLB
      Port: {{ $e.Port }}
      Protocol: {{ $e.Protocol }}
{{- end }}
Outputs:
  TargetGroupARN:
    Description: The Loadbalancer's TargetGroup ARN
    Value: !Ref ExocompTG
  LoadBalancerDNSName:
    Description: The DNS name for the load balancer
    Value: !GetAtt ExocompNLB.DNSName`

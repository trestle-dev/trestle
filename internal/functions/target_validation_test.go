package functions

import "testing"

// TestFunctionTargetValidation proves function targets are validated to the
// aws-lambda ARN shape and region pattern before outbound invocation.
func TestFunctionTargetValidation(t *testing.T) {
	if err := validateTarget("arn:aws:lambda:us-east-1:1:function:x", "us-east-1"); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	for _, bad := range []struct{ target, region string }{
		{"arn:aws:lambda:us-east-1:1:function:x", "eu-west-1"},
		{"not-an-arn", "us-east-1"},
		{"arn:aws:lambda:us-east-1:1:notfunction:x", "us-east-1"},
		{"arn:aws:ec2:us-east-1:1:function:x", "us-east-1"},
	} {
		if err := validateTarget(bad.target, bad.region); err == nil {
			t.Errorf("invalid target accepted: %s %s", bad.target, bad.region)
		}
	}
}

package functions

import "testing"

func FuzzLambdaTargetValidation(f *testing.F) {
	for _, seed := range [][2]string{{"arn:aws:lambda:ap-southeast-2:123456789012:function:alerts", "ap-southeast-2"}, {"", ""}, {"../../", "localhost"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, target, region string) { _ = validateTarget(target, region) })
}

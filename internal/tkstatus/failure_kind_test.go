// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tkstatus

import "testing"

func boolp(b bool) *bool { return &b }

// Test Kitchen names the phase that failed in its own output. That is what
// tells a cookbook that will not converge apart from a lab that could not
// build the VM to converge on — the two are otherwise stored identically:
// passed = false, no timeout, no error message.
//
// The sample outputs below are trimmed from real runs.

const createFailedOutput = `-----> Starting Test Kitchen (v4.0.0)
-----> Creating <default-almalinux-9>...
       Resolved template 'alma-template' to VMID 117 on node node-a
       Creating VM from template 117...
-----> Destroying <default-almalinux-9>...
>>>>>> ------Exception-------
>>>>>> Class: Kitchen::ActionFailed
>>>>>> Message: 1 actions failed.
>>>>>>     Failed to complete #create action: [Task timeout after 300s: UPID:qmclone:117] on default-almalinux-9
>>>>>> ----------------------`

const convergeFailedOutput = `-----> Starting Test Kitchen (v4.0.0)
-----> Converging <default-rocky-9>...
       Starting Chef Infra Client, version 19.3.15
       resolving cookbooks for run list: ["example::default"]
       Converging 3 resources
       * package[nginx] action install
       ================================================================================
       Error executing action ` + "`install`" + ` on resource 'package[nginx]'
>>>>>> ------Exception-------
>>>>>> Class: Kitchen::ActionFailed
>>>>>> Message: 1 actions failed.
>>>>>>     Failed to complete #converge action: [package install failed] on default-rocky-9
>>>>>> ----------------------`

const verifyFailedOutput = `-----> Converging <default-rocky-9>...
       Starting Chef Infra Client, version 19.3.15
       Converging 3 resources
-----> Verifying <default-rocky-9>...
       Failures:
         1) service nginx should be enabled
>>>>>> ------Exception-------
>>>>>> Message: 1 actions failed.
>>>>>>     Failed to complete #verify action: [Expected exit code 0] on default-rocky-9
>>>>>> ----------------------`

// The instance never existed, so no phase marker names a cookbook phase.
const noConvergeOutput = `-----> Starting Test Kitchen (v4.0.0)
-----> Cleaning up any prior instances of <default-centos-stream-9>
>>>>>> ------Exception-------
>>>>>> Class: Kitchen::UserError
>>>>>> Message: Cannot use remote lifecycle hooks during phases when the instance is not available
>>>>>> ----------------------`

func TestClassifyFailure(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		passed         *bool
		timedOut       bool
		networkTimeout bool
		want           string
		wantEvidence   bool
	}{
		{
			name: "a run that passed has no failure to explain",
			// A pass is a verdict about the cookbook, but not a failure kind.
			output: convergeFailedOutput, passed: boolp(true),
			want: FailureNone, wantEvidence: false,
		},
		{
			name:   "the lab could not build the VM, so nothing was learned about the cookbook",
			output: createFailedOutput, passed: boolp(false),
			want: FailureCreate, wantEvidence: false,
		},
		{
			name:   "the cookbook was converged and did not come up",
			output: convergeFailedOutput, passed: boolp(false),
			want: FailureConverge, wantEvidence: true,
		},
		{
			name:   "the cookbook converged but its own tests failed",
			output: verifyFailedOutput, passed: boolp(false),
			want: FailureVerify, wantEvidence: true,
		},
		{
			name:   "the run died before Chef ever started",
			output: noConvergeOutput, passed: boolp(false),
			want: FailureNoConverge, wantEvidence: false,
		},
		{
			name:   "a timeout with no sign of Chef starting is the network, not the cookbook",
			output: noConvergeOutput, timedOut: true, networkTimeout: true,
			want: FailureNetworkTimeout, wantEvidence: false,
		},
		{
			name:   "a timeout while converging is still not a verdict",
			output: convergeFailedOutput, timedOut: true,
			want: FailureTimeout, wantEvidence: false,
		},
		{
			// Never silently unblock something we cannot explain: an
			// unrecognised failure keeps counting as one.
			name:   "a failure we cannot explain still counts against the cookbook",
			output: "converging 3 resources\nsomething went wrong in a way nobody has seen",
			passed: boolp(false),
			want:   FailureUnknown, wantEvidence: true,
		},
		{
			name:   "a create failure is recognised even when the run also timed out later",
			output: createFailedOutput, passed: boolp(false), timedOut: false,
			want: FailureCreate, wantEvidence: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyFailure(tt.output, tt.passed, tt.timedOut, tt.networkTimeout)
			if got != tt.want {
				t.Errorf("ClassifyFailure = %q, want %q", got, tt.want)
			}
			if ev := CountsAsCookbookFailure(tt.passed, got); ev != tt.wantEvidence {
				t.Errorf("CountsAsCookbookFailure(%q) = %v, want %v", got, ev, tt.wantEvidence)
			}
		})
	}
}

// A converge failure that also failed to tear down is still a cookbook
// failure — the cookbook verdict outranks the lab's tidying up.
func TestClassifyFailure_ConvergeBeatsDestroy(t *testing.T) {
	out := convergeFailedOutput + "\n>>>>>>     Failed to complete #destroy action: [connection refused] on default-rocky-9"
	if got := ClassifyFailure(out, boolp(false), false, false); got != FailureConverge {
		t.Errorf("ClassifyFailure = %q, want %q", got, FailureConverge)
	}
}

// Tearing down is the lab's problem. It leaks a VM, but it says nothing about
// whether the cookbook converged.
func TestClassifyFailure_DestroyIsNotCookbookEvidence(t *testing.T) {
	out := `-----> Converging <default-rocky-9>...
       Converging 3 resources
>>>>>>     Failed to complete #destroy action: [connection refused] on default-rocky-9`
	got := ClassifyFailure(out, boolp(false), false, false)
	if got != FailureDestroy {
		t.Fatalf("ClassifyFailure = %q, want %q", got, FailureDestroy)
	}
	if CountsAsCookbookFailure(boolp(false), got) {
		t.Errorf("a destroy failure must not count as a verdict about the cookbook")
	}
}

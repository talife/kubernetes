/*
Copyright 2017 The Kubernetes Authors.

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

package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kubeadmapiv1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta4"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

func TestSessionIsInteractive(t *testing.T) {
	var tcases = []struct {
		name     string
		data     *applyData
		expected bool
	}{
		{
			name: "Explicitly non-interactive",
			data: &applyData{
				nonInteractiveMode: true,
			},
			expected: false,
		},
		{
			name: "Implicitly non-interactive since --dryRun is used",
			data: &applyData{
				dryRun: true,
			},
			expected: false,
		},
		{
			name: "Implicitly non-interactive since --force is used",
			data: &applyData{
				force: true,
			},
			expected: false,
		},
		{
			name:     "Interactive session",
			data:     &applyData{},
			expected: true,
		},
	}
	for _, tt := range tcases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.data.SessionIsInteractive() != tt.expected {
				t.Error("unexpected result")
			}
		})
	}
}

var testApplyConfig = fmt.Sprintf(`---
apiVersion: %s
apply:
  certificateRenewal: true
  etcdUpgrade: true
  imagePullPolicy: IfNotPresent
  imagePullSerial: true
diff: {}
kind: UpgradeConfiguration
node:
  certificateRenewal: true
  etcdUpgrade: true
  imagePullPolicy: IfNotPresent
  imagePullSerial: true
plan: {}
timeouts:
  controlPlaneComponentHealthCheck: 4m0s
  discovery: 5m0s
  etcdAPICall: 2m0s
  kubeletHealthCheck: 4m0s
  kubernetesAPICall: 1m0s
  tlsBootstrap: 5m0s
  upgradeManifests: 5m0s
`, kubeadmapiv1.SchemeGroupVersion.String())

func TestNewApplyData(t *testing.T) {
	// create temp directory
	tmpDir, err := os.MkdirTemp("", "kubeadm-upgrade-apply-test")
	if err != nil {
		t.Errorf("Unable to create temporary directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// create config file
	configFilePath := filepath.Join(tmpDir, "test-config-file")
	cfgFile, err := os.Create(configFilePath)
	if err != nil {
		t.Errorf("Unable to create file %q: %v", configFilePath, err)
	}
	defer func() {
		_ = cfgFile.Close()
	}()
	if _, err = cfgFile.WriteString(testApplyConfig); err != nil {
		t.Fatalf("Unable to write file %q: %v", configFilePath, err)
	}

	testCases := []struct {
		name          string
		args          []string
		flags         map[string]string
		validate      func(*testing.T, *applyData)
		expectedError string
	}{
		{
			name: "fails if no upgrade version set",
			flags: map[string]string{
				options.CfgPath: configFilePath,
			},
			expectedError: "missing one or more required arguments. Required arguments: [version]",
		},
		{
			name: "fails if invalid preflight checks are provided",
			args: []string{"v1.1.0"},
			flags: map[string]string{
				options.IgnorePreflightErrors: "all,something-else",
			},
			expectedError: "ignore-preflight-errors: Invalid value",
		},
		{
			name: "fails if kubeconfig file doesn't exists",
			args: []string{"v1.1.0"},
			flags: map[string]string{
				options.CfgPath:        configFilePath,
				options.KubeconfigPath: "invalid-kubeconfig-path",
			},
			expectedError: "couldn't create a Kubernetes client from file",
		},

		// TODO: add more test cases here when the fake client for `kubeadm upgrade apply` can be injected
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// initialize an external apply flags and inject it to the apply cmd
			apf := &applyPlanFlags{
				kubeConfigPath:            kubeadmconstants.GetAdminKubeConfigPath(),
				cfgPath:                   "",
				featureGatesString:        "",
				allowExperimentalUpgrades: false,
				allowRCUpgrades:           false,
				printConfig:               false,
				out:                       os.Stdout,
			}

			cmd := newCmdApply(apf)

			// sets cmd flags (that will be reflected on the init options)
			for f, v := range tc.flags {
				_ = cmd.Flags().Set(f, v)
			}

			flags := &applyFlags{
				applyPlanFlags: apf,
				etcdUpgrade:    true,
				renewCerts:     true,
			}

			// test newApplyData method
			data, err := newApplyData(cmd, tc.args, flags)
			if err == nil && len(tc.expectedError) != 0 {
				t.Error("Expected error, but got success")
			}
			if err != nil && (len(tc.expectedError) == 0 || !strings.Contains(err.Error(), tc.expectedError)) {
				t.Fatalf("newApplyData returned unexpected error, expected: %s, got %v", tc.expectedError, err)
			}

			// exec additional validation on the returned value
			if tc.validate != nil {
				tc.validate(t, data)
			}
		})
	}
}

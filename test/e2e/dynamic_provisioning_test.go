/*
Copyright 2020 The Kubernetes Authors.

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

package e2e

import (
	"fmt"

	"github.com/kubernetes-csi/csi-driver-smb/test/e2e/driver"
	"github.com/kubernetes-csi/csi-driver-smb/test/e2e/testsuites"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientset "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	restclientset "k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = ginkgo.Describe("Dynamic Provisioning", func() {
	f := framework.NewDefaultFramework("smb")

	var (
		cs         clientset.Interface
		ns         *v1.Namespace
		testDriver driver.PVTestDriver
	)

	ginkgo.BeforeEach(func() {
		checkPodsRestart := testCmd{
			command:  "sh",
			args:     []string{"test/utils/check_driver_pods_restart.sh"},
			startLog: "Check driver pods if restarts ...",
			endLog:   "Check successfully",
		}
		execTestCmd([]testCmd{checkPodsRestart})

		cs = f.ClientSet
		ns = f.Namespace

		var err error
		_, err = restClient(testsuites.SnapshotAPIGroup, testsuites.APIVersionv1beta1)
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("could not get rest clientset: %v", err))
		}
	})

	testDriver = driver.InitSMBDriver()
	ginkgo.It("should create a volume after driver restart [smb.csi.k8s.io]", func() {
		ginkgo.Skip("test case is disabled since node logs would be lost after driver restart")
		pod := testsuites.PodDetails{
			Cmd: convertToPowershellCommandIfNecessary("echo 'hello world' >> /mnt/test-1/data && while true; do sleep 3600; done"),
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
			IsWindows: isWindowsCluster,
		}

		podCheckCmd := []string{"cat", "/mnt/test-1/data"}
		expectedString := "hello world\n"
		if isWindowsCluster {
			podCheckCmd = []string{"powershell.exe", "-Command", "Get-Content C:\\mnt\\test-1\\data.txt"}
			expectedString = "hello world\r\n"
		}
		test := testsuites.DynamicallyProvisionedRestartDriverTest{
			CSIDriver: testDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            podCheckCmd,
				ExpectedString: expectedString,
			},
			StorageClassParameters: defaultStorageClassParameters,
			RestartDriverFunc: func() {
				restartDriver := testCmd{
					command:  "bash",
					args:     []string{"test/utils/restart_driver_daemonset.sh"},
					startLog: "Restart driver node daemonset ...",
					endLog:   "Restart driver node daemonset done successfully",
				}
				execTestCmd([]testCmd{restartDriver})
			},
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a volume on demand with mount options [smb.csi.k8s.io] [Windows]", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: convertToPowershellCommandIfNecessary("echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"dir_mode=0777",
							"file_mode=0777",
							"uid=0",
							"gid=0",
							"mfsymlinks",
							"cache=strict",
							"nosharesock",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: defaultStorageClassParameters,
		}

		test.Run(cs, ns)
	})

	ginkgo.It("should create multiple PV objects, bind to PVCs and attach all to different pods on the same node [smb.csi.k8s.io] [Windows]", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: convertToPowershellCommandIfNecessary("while true; do echo $(date -u) >> /mnt/test-1/data; sleep 100; done"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
			{
				Cmd: convertToPowershellCommandIfNecessary("while true; do echo $(date -u) >> /mnt/test-1/data; sleep 100; done"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
		}
		test := testsuites.DynamicallyProvisionedCollocatedPodTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			ColocatePods:           true,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("[createSubDir] should create multiple PV objects, bind to PVCs and attach all to different pods on the same node [smb.csi.k8s.io] [Windows]", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: convertToPowershellCommandIfNecessary("while true; do echo $(date -u) >> /mnt/test-1/data; sleep 100; done"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
			{
				Cmd: convertToPowershellCommandIfNecessary("while true; do echo $(date -u) >> /mnt/test-1/data; sleep 100; done"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
		}
		test := testsuites.DynamicallyProvisionedCollocatedPodTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			ColocatePods:           true,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	// Track issue https://github.com/kubernetes/kubernetes/issues/70505
	ginkgo.It("should create a volume on demand and mount it as readOnly in a pod [smb.csi.k8s.io]", func() {
		// Windows volume does not support readOnly
		skipIfTestingInWindowsCluster()
		pods := []testsuites.PodDetails{
			{
				Cmd: convertToPowershellCommandIfNecessary("touch /mnt/test-1/data"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a deployment object, write and read to it, delete the pod and write and read to it again [smb.csi.k8s.io] [Windows]", func() {
		pod := testsuites.PodDetails{
			Cmd: convertToPowershellCommandIfNecessary("echo 'hello world' >> /mnt/test-1/data && while true; do sleep 100; done"),
			Volumes: []testsuites.VolumeDetails{
				{
					ClaimSize: "10Gi",
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
			IsWindows: isWindowsCluster,
		}

		podCheckCmd := []string{"cat", "/mnt/test-1/data"}
		expectedString := "hello world\n"
		if isWindowsCluster {
			podCheckCmd = []string{"powershell.exe", "-Command", "Get-Content C:\\mnt\\test-1\\data.txt"}
			expectedString = "hello world\r\n"
		}
		test := testsuites.DynamicallyProvisionedDeletePodTest{
			CSIDriver: testDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            podCheckCmd,
				ExpectedString: expectedString, // pod will be restarted so expect to see 2 instances of string
			},
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It(fmt.Sprintf("should delete PV with reclaimPolicy %q [smb.csi.k8s.io] [Windows]", v1.PersistentVolumeReclaimDelete), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		volumes := []testsuites.VolumeDetails{
			{
				ClaimSize:     "10Gi",
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver:              testDriver,
			Volumes:                volumes,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It(fmt.Sprintf("should retain PV with reclaimPolicy %q [smb.csi.k8s.io] [Windows]", v1.PersistentVolumeReclaimRetain), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumes := []testsuites.VolumeDetails{
			{
				ClaimSize:     "10Gi",
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver:              testDriver,
			Volumes:                volumes,
			Driver:                 smbDriver,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a pod with multiple volumes [smb.csi.k8s.io] [Windows]", func() {
		volumes := []testsuites.VolumeDetails{}
		for i := 1; i <= 6; i++ {
			volume := testsuites.VolumeDetails{
				ClaimSize: "100Gi",
				VolumeMount: testsuites.VolumeMountDetails{
					NameGenerate:      "test-volume-",
					MountPathGenerate: "/mnt/test-",
				},
			}
			volumes = append(volumes, volume)
		}

		pods := []testsuites.PodDetails{
			{
				Cmd:       convertToPowershellCommandIfNecessary("echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data"),
				Volumes:   volumes,
				IsWindows: isWindowsCluster,
			},
		}
		test := testsuites.DynamicallyProvisionedPodWithMultiplePVsTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a pod with volume mount subpath [smb.csi.k8s.io] [Windows]", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: convertToPowershellCommandIfNecessary("echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data"),
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
				IsWindows: isWindowsCluster,
			},
		}
		test := testsuites.DynamicallyProvisionedVolumeSubpathTester{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: defaultStorageClassParameters,
		}
		test.Run(cs, ns)
	})
})

func restClient(group string, version string) (restclientset.Interface, error) {
	config, err := framework.LoadConfig()
	if err != nil {
		ginkgo.Fail(fmt.Sprintf("could not load config: %v", err))
	}
	gv := schema.GroupVersion{Group: group, Version: version}
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(runtime.NewScheme())}
	return restclientset.RESTClientFor(config)
}

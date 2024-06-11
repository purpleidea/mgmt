package coreos

import (
	"path/filepath"
	"testing"
)

func TestMountInfoScanning(t *testing.T) {
	tests := []struct {
		name            string
		mountInfo       string
		target          string
		expectContainer bool
	}{
		{
			name:            "Alpine Docker",
			mountInfo:       "docker_alpine_cgroup2_mountinfo",
			target:          "docker/containers",
			expectContainer: true,
		},
		{
			name:            "Ubuntu Docker",
			mountInfo:       "docker_ubuntu_cgroup2_mountinfo",
			target:          "docker/containers",
			expectContainer: true,
		},
		{
			name:            "Ubuntu Metal",
			mountInfo:       "ubuntu_22.04_mountinfo",
			target:          "docker/containers",
			expectContainer: false,
		},
		{
			name:            "LXC Ubuntu",
			mountInfo:       "lxc_proc_1_environ",
			target:          "container=lxc",
			expectContainer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docker_mount := filepath.Join("resources", tt.mountInfo)
			isContainer := scanFile(fileTarget{file: docker_mount, target: tt.target})

			if tt.expectContainer {
				if !isContainer {
					t.Fatalf("Failed to locate proc mounts")
				}
			} else {
				if isContainer {
					t.Fatalf("False detection")
				}
			}
		})
	}
}

func TestAWSSysScanning(t *testing.T) {
	tests := []struct {
		name        string
		sysInfoFile string
		target      string
		isEc2       bool
	}{
		{
			name:        "AWS Board ID",
			sysInfoFile: "aws_sys_class_dmi_id_board_vendor",
			target:      "Amazon EC2",
			isEc2:       true,
		},
		{
			name:        "AWS Vendor Id",
			sysInfoFile: "aws_sys_class_dmi_id_sys_vendor",
			target:      "Amazon EC2",
			isEc2:       true,
		},
		{
			name:        "Not AWS",
			sysInfoFile: "aws_empty",
			target:      "Amazon EC2",
			isEc2:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dmiFile := filepath.Join("resources", tt.sysInfoFile)
			targetLocated := scanFile(fileTarget{file: dmiFile, target: tt.target})

			if tt.isEc2 {
				if !targetLocated {
					t.Fatalf("Failed to locate sys files")
				}
			} else {
				if targetLocated {
					t.Fatalf("False detection")
				}
			}

			targetFiles := []fileTarget{{file: filepath.Join("resources", tt.sysInfoFile), target: tt.target}}

			mockEc2 := ConcretePlatform{
				name:        VIRTUALIZATION_EC2,
				fileTargets: targetFiles,
			}

			isEc2 := mockEc2.evaluate()

			if tt.isEc2 {
				if isEc2 != VIRTUALIZATION_EC2 {
					t.Fatalf("Failed to detect as ec2")
				}
			} else {
				if isEc2 == VIRTUALIZATION_EC2 {
					t.Fatalf("False detection")
				}
			}
		})
	}
}

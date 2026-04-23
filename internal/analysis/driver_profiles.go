// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "sort"

// DriverProfile describes a built-in Test Kitchen driver's image field
// mapping and common secret names.
type DriverProfile struct {
	// Name is the driver name as configured (e.g. "vcenter", "ec2").
	Name string

	// ImageFieldName is the YAML key used in platform entries for the
	// driver-specific image identifier (e.g. "template" for vcenter,
	// "ami" for ec2).
	ImageFieldName string

	// TypicalSecrets lists the common secret key names for this driver.
	// Informational only — not used for validation.
	TypicalSecrets []string
}

var builtinProfiles = map[string]DriverProfile{
	"vcenter": {Name: "vcenter", ImageFieldName: "template", TypicalSecrets: []string{"vcenter_password"}},
	"vra":     {Name: "vra", ImageFieldName: "image_mapping", TypicalSecrets: []string{"password"}},
	"ec2":     {Name: "ec2", ImageFieldName: "ami", TypicalSecrets: []string{"aws_secret_access_key"}},
	"vagrant": {Name: "vagrant", ImageFieldName: "box", TypicalSecrets: nil},
	"proxmox": {Name: "proxmox", ImageFieldName: "template_id", TypicalSecrets: []string{"proxmox_token_secret"}},
}

// LookupProfile returns the driver profile for the given driver name.
// For built-in profiles, the imageFieldNameOverride is ignored.
// For unknown drivers, imageFieldNameOverride is used as the ImageFieldName.
// Returns a valid profile in all cases.
func LookupProfile(driverName, imageFieldNameOverride string) DriverProfile {
	if p, ok := builtinProfiles[driverName]; ok {
		return p
	}
	fieldName := imageFieldNameOverride
	if fieldName == "" {
		fieldName = "image"
	}
	return DriverProfile{
		Name:           driverName,
		ImageFieldName: fieldName,
	}
}

// IsBuiltinDriver returns true if the driver name matches a built-in profile.
func IsBuiltinDriver(driverName string) bool {
	_, ok := builtinProfiles[driverName]
	return ok
}

// BuiltinDriverNames returns a sorted list of all built-in driver names.
func BuiltinDriverNames() []string {
	names := make([]string, 0, len(builtinProfiles))
	for k := range builtinProfiles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

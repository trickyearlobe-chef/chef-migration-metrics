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
    // "ami" for ec2, "docker_image" for dokken).
    ImageFieldName string

    // TypicalSecrets lists the common secret key names for this driver.
    // Informational only — not used for validation.
    TypicalSecrets []string
}

var builtinProfiles = map[string]DriverProfile{
    "dokken":    {Name: "dokken", ImageFieldName: "docker_image", TypicalSecrets: nil},
    "vcenter":   {Name: "vcenter", ImageFieldName: "template", TypicalSecrets: []string{"vcenter_password"}},
    "vra":       {Name: "vra", ImageFieldName: "image_mapping", TypicalSecrets: []string{"password"}},
    "ec2":       {Name: "ec2", ImageFieldName: "ami", TypicalSecrets: []string{"aws_secret_access_key"}},
    "azurerm":   {Name: "azurerm", ImageFieldName: "image_urn", TypicalSecrets: []string{"client_secret"}},
    "google":    {Name: "google", ImageFieldName: "image_family", TypicalSecrets: []string{"service_account_json"}},
    "vagrant":   {Name: "vagrant", ImageFieldName: "box", TypicalSecrets: nil},
    "openstack": {Name: "openstack", ImageFieldName: "image_ref", TypicalSecrets: []string{"os_password"}},
    "proxmox":   {Name: "proxmox", ImageFieldName: "template", TypicalSecrets: []string{"proxmox_password"}},
}

// LookupProfile returns the driver profile for the given driver name.
// For built-in profiles, the imageFieldNameOverride is ignored.
// For the "custom" profile or unknown drivers, imageFieldNameOverride is
// used as the ImageFieldName. Returns a valid profile in all cases.
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

// IsDokken returns true if the driver name is "dokken".
func IsDokken(driverName string) bool {
    return driverName == "" || driverName == "dokken"
}

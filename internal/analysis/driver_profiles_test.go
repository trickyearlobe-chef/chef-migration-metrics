// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
    "sort"
    "testing"
)

func TestLookupProfile_VCenter(t *testing.T) {
    p := LookupProfile("vcenter", "")
    if p.ImageFieldName != "template" {
        t.Errorf("expected ImageFieldName %q, got %q", "template", p.ImageFieldName)
    }
    if p.Name != "vcenter" {
        t.Errorf("expected Name %q, got %q", "vcenter", p.Name)
    }
}

func TestLookupProfile_Proxmox(t *testing.T) {
    p := LookupProfile("proxmox", "")
    if p.ImageFieldName != "template_name" {
        t.Errorf("expected ImageFieldName %q, got %q", "template_name", p.ImageFieldName)
    }
    if p.Name != "proxmox" {
        t.Errorf("expected Name %q, got %q", "proxmox", p.Name)
    }
}

func TestLookupProfile_EC2(t *testing.T) {
    p := LookupProfile("ec2", "")
    if p.ImageFieldName != "ami" {
        t.Errorf("expected ImageFieldName %q, got %q", "ami", p.ImageFieldName)
    }
    if p.Name != "ec2" {
        t.Errorf("expected Name %q, got %q", "ec2", p.Name)
    }
}

func TestLookupProfile_AllBuiltins(t *testing.T) {
    for _, name := range BuiltinDriverNames() {
        p := LookupProfile(name, "")
        if p.ImageFieldName == "" {
            t.Errorf("built-in driver %q returned empty ImageFieldName", name)
        }
        if p.Name != name {
            t.Errorf("built-in driver %q: expected Name %q, got %q", name, name, p.Name)
        }
    }
}

func TestLookupProfile_Custom(t *testing.T) {
    p := LookupProfile("custom", "my_image")
    if p.ImageFieldName != "my_image" {
        t.Errorf("expected ImageFieldName %q, got %q", "my_image", p.ImageFieldName)
    }
    if p.Name != "custom" {
        t.Errorf("expected Name %q, got %q", "custom", p.Name)
    }
}

func TestLookupProfile_UnknownWithOverride(t *testing.T) {
    p := LookupProfile("mydriver", "template_name")
    if p.ImageFieldName != "template_name" {
        t.Errorf("expected ImageFieldName %q, got %q", "template_name", p.ImageFieldName)
    }
    if p.Name != "mydriver" {
        t.Errorf("expected Name %q, got %q", "mydriver", p.Name)
    }
}

func TestLookupProfile_UnknownWithoutOverride(t *testing.T) {
    p := LookupProfile("mydriver", "")
    if p.ImageFieldName != "image" {
        t.Errorf("expected ImageFieldName %q, got %q", "image", p.ImageFieldName)
    }
}

func TestLookupProfile_BuiltinIgnoresOverride(t *testing.T) {
    // Built-in profiles must ignore the imageFieldNameOverride parameter.
    builtins := map[string]string{
        "vcenter": "template",
        "ec2":     "ami",
        "vagrant": "box",
        "proxmox": "template_name",
        "vra":     "image_mapping",
    }
    for driver, expectedField := range builtins {
        p := LookupProfile(driver, "my_custom_override")
        if p.ImageFieldName != expectedField {
            t.Errorf("LookupProfile(%q, \"my_custom_override\"): expected ImageFieldName %q, got %q — override should be ignored for built-in drivers",
                driver, expectedField, p.ImageFieldName)
        }
    }
}

func TestIsBuiltinDriver_Known(t *testing.T) {
    known := []string{"vcenter", "vra", "ec2", "vagrant", "proxmox"}
    for _, name := range known {
        if !IsBuiltinDriver(name) {
            t.Errorf("expected IsBuiltinDriver(%q) to be true", name)
        }
    }
}

func TestIsBuiltinDriver_Unknown(t *testing.T) {
    unknown := []string{"custom", "mydriver"}
    for _, name := range unknown {
        if IsBuiltinDriver(name) {
            t.Errorf("expected IsBuiltinDriver(%q) to be false", name)
        }
    }
}

func TestBuiltinDriverNames_Sorted(t *testing.T) {
    names := BuiltinDriverNames()
    if len(names) == 0 {
        t.Fatal("expected non-empty list of built-in driver names")
    }
    if !sort.StringsAreSorted(names) {
        t.Errorf("expected sorted list, got %v", names)
    }
}

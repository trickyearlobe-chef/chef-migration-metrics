// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package hypervisor provides an abstraction layer for hypervisor
// operations needed by CMM: template discovery, VM inventory, and
// orphan cleanup. Implementations exist for vCenter (production) and
// Proxmox (proof-of-concept).
package hypervisor

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxVMNameLen is the vCenter VM name length limit.
const maxVMNameLen = 80

// sanitiseRe matches any character that is not a lowercase letter or digit.
var sanitiseRe = regexp.MustCompile(`[^a-z0-9]+`)

// Hypervisor abstracts hypervisor-specific operations. CMM does not manage
// VMs directly (Test Kitchen does that via kitchen-vcenter etc.). CMM needs
// three things from the hypervisor: template discovery, VM inventory, and
// VM destruction (for orphan cleanup).
type Hypervisor interface {
	// ListTemplates returns available VM templates from the hypervisor.
	ListTemplates(ctx context.Context) ([]Template, error)

	// ListManagedVMs returns VMs matching the CMM naming convention.
	// The prefix parameter filters to VMs whose name starts with the given prefix.
	ListManagedVMs(ctx context.Context, prefix string) ([]ManagedVM, error)

	// DestroyVM force-destroys a VM by its hypervisor-specific identifier.
	// It should power off the VM if running, then delete it.
	DestroyVM(ctx context.Context, hypervisorID string) error

	// Type returns the hypervisor type name (e.g. "vcenter", "proxmox").
	Type() string
}

// Template represents a VM template available on the hypervisor.
type Template struct {
	// ID is the hypervisor-specific template identifier.
	ID string `json:"id"`

	// Name is the human-readable template name.
	Name string `json:"name"`

	// GuestOS is the guest operating system type (e.g. "rhel8_64Guest").
	GuestOS string `json:"guest_os,omitempty"`

	// Notes contains any description/annotation on the template.
	Notes string `json:"notes,omitempty"`

	// LastModified is when the template was last updated.
	LastModified time.Time `json:"last_modified,omitempty"`
}

// ManagedVM represents a VM that matches the CMM naming convention.
type ManagedVM struct {
	// HypervisorID is the hypervisor-specific VM identifier
	// (e.g. vCenter MoRef, Proxmox VMID).
	HypervisorID string `json:"hypervisor_id"`

	// Name is the full VM name.
	Name string `json:"name"`

	// PowerState is the current power state:
	// "poweredOn", "poweredOff", "suspended".
	PowerState string `json:"power_state"`

	// CPUCount is the number of vCPUs allocated.
	CPUCount int `json:"cpu_count,omitempty"`

	// MemoryMB is the memory allocated in megabytes.
	MemoryMB int `json:"memory_mb,omitempty"`

	// CreatedAt is the VM creation time (if available from hypervisor).
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// VMStatus represents the lifecycle status of a CMM-tracked VM.
type VMStatus string

const (
	VMStatusCreating   VMStatus = "creating"
	VMStatusRunning    VMStatus = "running"
	VMStatusDestroying VMStatus = "destroying"
	VMStatusDestroyed  VMStatus = "destroyed"
	VMStatusOrphaned   VMStatus = "orphaned"
)

// ValidVMStatuses is the set of valid VM status values.
var ValidVMStatuses = map[VMStatus]bool{
	VMStatusCreating:   true,
	VMStatusRunning:    true,
	VMStatusDestroying: true,
	VMStatusDestroyed:  true,
	VMStatusOrphaned:   true,
}

// VMNameComponents holds the parsed parts of a CMM VM name.
type VMNameComponents struct {
	Prefix       string
	CookbookName string
	SuiteName    string
	PlatformName string
	Timestamp    int64
}

// sanitiseComponent lowercases the input, replaces non-alphanumeric runs
// with a single hyphen, and trims leading/trailing hyphens.
func sanitiseComponent(s string) string {
	s = strings.ToLower(s)
	s = sanitiseRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// GenerateVMName creates a VM name following the CMM convention:
//
//	{prefix}-{cookbook}-{suite}-{platform}-{unix-timestamp}
//
// Components are sanitised: lowercased, non-alphanumeric chars replaced
// with hyphens, consecutive hyphens collapsed, truncated to keep total
// name under 80 chars (vCenter limit).
func GenerateVMName(prefix, cookbookName, suiteName, platformName string, ts time.Time) string {
	if prefix == "" {
		prefix = "cmm"
	}
	prefix = sanitiseComponent(prefix)
	cookbook := sanitiseComponent(cookbookName)
	suite := sanitiseComponent(suiteName)
	platform := sanitiseComponent(platformName)
	timestamp := strconv.FormatInt(ts.Unix(), 10)

	// Calculate overhead: 4 hyphens separating 5 parts.
	overhead := len(prefix) + 1 + 1 + len(suite) + 1 + len(platform) + 1 + len(timestamp)
	maxCookbook := maxVMNameLen - overhead
	if maxCookbook < 1 {
		maxCookbook = 1
	}
	if len(cookbook) > maxCookbook {
		cookbook = strings.TrimRight(cookbook[:maxCookbook], "-")
	}

	var b strings.Builder
	b.Grow(maxVMNameLen)
	b.WriteString(prefix)
	b.WriteByte('-')
	b.WriteString(cookbook)
	b.WriteByte('-')
	b.WriteString(suite)
	b.WriteByte('-')
	b.WriteString(platform)
	b.WriteByte('-')
	b.WriteString(timestamp)
	return b.String()
}

// ParseVMName attempts to extract components from a CMM VM name.
// The prefix must be known (passed in). The timestamp is the last
// numeric segment. The middle is split into cookbook/suite/platform
// by position (first=cookbook, second=suite, rest=platform).
// Returns ok=false if the name doesn't start with the prefix or
// has no valid timestamp suffix.
func ParseVMName(name, expectedPrefix string) (VMNameComponents, bool) {
	expectedPrefix = sanitiseComponent(expectedPrefix)
	if expectedPrefix == "" {
		expectedPrefix = "cmm"
	}
	if !strings.HasPrefix(name, expectedPrefix+"-") {
		return VMNameComponents{}, false
	}

	// Strip prefix.
	rest := name[len(expectedPrefix)+1:]

	// The last segment separated by "-" must be a numeric timestamp
	// of at least 10 digits.
	lastHyphen := strings.LastIndex(rest, "-")
	if lastHyphen < 0 {
		return VMNameComponents{}, false
	}
	tsPart := rest[lastHyphen+1:]
	if len(tsPart) < 10 {
		return VMNameComponents{}, false
	}
	ts, err := strconv.ParseInt(tsPart, 10, 64)
	if err != nil {
		return VMNameComponents{}, false
	}

	middle := rest[:lastHyphen]
	if middle == "" {
		return VMNameComponents{}, false
	}

	// Split middle into at most 3 parts by first two hyphens.
	parts := strings.SplitN(middle, "-", 3)
	comp := VMNameComponents{
		Prefix:    expectedPrefix,
		Timestamp: ts,
	}
	switch len(parts) {
	case 1:
		comp.CookbookName = parts[0]
	case 2:
		comp.CookbookName = parts[0]
		comp.SuiteName = parts[1]
	default:
		comp.CookbookName = parts[0]
		comp.SuiteName = parts[1]
		comp.PlatformName = parts[2]
	}
	return comp, true
}

// NullHypervisor is a no-op implementation used when no hypervisor is
// configured.
type NullHypervisor struct{}

func (NullHypervisor) ListTemplates(_ context.Context) ([]Template, error) {
	return nil, nil
}

func (NullHypervisor) ListManagedVMs(_ context.Context, _ string) ([]ManagedVM, error) {
	return nil, nil
}

func (NullHypervisor) DestroyVM(_ context.Context, id string) error {
	return fmt.Errorf("hypervisor: no hypervisor configured — cannot destroy VM %s", id)
}

func (NullHypervisor) Type() string { return "none" }

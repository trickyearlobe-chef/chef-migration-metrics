// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// virtualFSTypes lists filesystem types that are virtual/pseudo and should
// be excluded by default (unless ?show_all=true is specified).
var virtualFSTypes = map[string]bool{
	"proc":          true,
	"sysfs":         true,
	"devpts":        true,
	"tmpfs":         true,
	"squashfs":      true,
	"cgroup":        true,
	"cgroup2":       true,
	"bpf":           true,
	"devtmpfs":      true,
	"debugfs":       true,
	"securityfs":    true,
	"pstore":        true,
	"configfs":      true,
	"fusectl":       true,
	"hugetlbfs":     true,
	"mqueue":        true,
	"tracefs":       true,
	"efivarfs":      true,
	"binfmt_misc":   true,
	"autofs":        true,
	"rpc_pipefs":    true,
	"nsfs":          true,
	"ramfs":         true,
	"overlay":       true,
	"fuse.snapfuse": true,
	"fuse.lxcfs":    true,
}

// DiskEntry represents a single parsed disk/mount entry from the Ohai
// filesystem data.
type DiskEntry struct {
	Mount             string   `json:"mount"`
	Device            string   `json:"device"`
	FSType            string   `json:"fs_type"`
	KBSize            int64    `json:"kb_size"`
	KBUsed            int64    `json:"kb_used"`
	KBAvailable       int64    `json:"kb_available"`
	PercentUsed       int64    `json:"percent_used"`
	UUID              string   `json:"uuid,omitempty"`
	MountOptions      []string `json:"mount_options,omitempty"`
	InodesUsed        *int64   `json:"inodes_used,omitempty"`
	TotalInodes       *int64   `json:"total_inodes,omitempty"`
	InodesAvailable   *int64   `json:"inodes_available,omitempty"`
	InodesPercentUsed *int64   `json:"inodes_percent_used,omitempty"`
	DriveType         string   `json:"drive_type,omitempty"`
	VolumeName        string   `json:"volume_name,omitempty"`
	EncryptionStatus  string   `json:"encryption_status,omitempty"`
}

// diskResponse is the JSON envelope returned by handleNodeDisks.
type diskResponse struct {
	NodeName         string      `json:"node_name"`
	OrganisationName string      `json:"organisation_name"`
	Platform         string      `json:"platform"`
	Disks            []DiskEntry `json:"disks"`
}

// handleNodeDisks handles GET /api/v1/nodes/disks/:org/:name — returns
// parsed filesystem/disk information for a single node.
func (r *Router) handleNodeDisks(w http.ResponseWriter, req *http.Request) {
	segs := pathSegments(req.URL.Path, "/api/v1/nodes/disks/")
	if len(segs) < 2 {
		WriteNotFound(w, "Node disks endpoint requires /api/v1/nodes/disks/:organisation/:name.")
		return
	}

	if !requireGET(w, req) {
		return
	}

	orgName := segs[0]
	nodeName := strings.Join(segs[1:], "/") // node names may contain slashes

	showAll := req.URL.Query().Get("show_all") == "true"

	// Resolve organisation by name.
	org, err := r.db.GetOrganisationByName(req.Context(), orgName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting organisation %s for disks: %v", orgName, err)
		WriteInternalError(w, "Failed to get organisation.")
		return
	}

	// Get the most recent snapshot for this node.
	snapshot, err := r.db.GetNodeSnapshotByName(req.Context(), org.Name, nodeName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Node %q not found in organisation %q.", nodeName, orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting node snapshot %s/%s for disks: %v", orgName, nodeName, err)
		WriteInternalError(w, "Failed to get node.")
		return
	}

	// Parse filesystem data.
	disks, err := parseFilesystemData(snapshot.Filesystem, showAll)
	if err != nil {
		r.logf("WARN", "parsing filesystem for %s/%s: %v", orgName, nodeName, err)
		// Return an empty disk list rather than failing.
		disks = []DiskEntry{}
	}

	WriteJSON(w, http.StatusOK, diskResponse{
		NodeName:         snapshot.NodeName,
		OrganisationName: org.Name,
		Platform:         snapshot.Platform,
		Disks:            disks,
	})
}

// parseFilesystemData extracts disk entries from Ohai filesystem JSONB.
//
// Two stored shapes are supported, because both coexist in the table until
// every organisation has been re-collected:
//
//   - Narrowed: the node partial search requests ["filesystem","by_pair"], so
//     the by_pair map is stored directly, keyed "device,mount".
//   - Legacy: the full Ohai wrapper with by_mountpoint / by_pair / by_device
//     sections nested inside.
//
// For pair-keyed maps the mount comes from the entry's "mount" field where
// present and from the key otherwise — some Ohai versions omit the redundant
// field, and requiring it left the page blank while disk verdicts still worked.
//
// When showAll is false, virtual/pseudo filesystems are filtered out.
func parseFilesystemData(raw json.RawMessage, showAll bool) ([]DiskEntry, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []DiskEntry{}, nil
	}

	byMount, err := filesystemEntriesByMount(raw)
	if err != nil {
		return nil, err
	}
	if byMount == nil {
		return []DiskEntry{}, nil
	}

	entries := make([]DiskEntry, 0, len(byMount))
	for mount, info := range byMount {
		fsType := stringVal(info["fs_type"])

		// Filter virtual filesystems unless show_all is requested.
		if !showAll && isVirtualFS(fsType, mount) {
			continue
		}

		kbSize := diskToInt64(info["kb_size"])
		kbUsed := diskToInt64(info["kb_used"])
		kbAvailable := diskToInt64(info["kb_available"])

		pctUsed := diskToInt64(info["percent_used"])
		if pctUsed <= 0 && kbSize > 0 {
			pctUsed = int64(math.Round(float64(kbUsed) / float64(kbSize) * 100))
		}

		entry := DiskEntry{
			Mount:       mount,
			Device:      deviceFromInfo(info),
			FSType:      fsType,
			KBSize:      kbSize,
			KBUsed:      kbUsed,
			KBAvailable: kbAvailable,
			PercentUsed: pctUsed,
			UUID:        stringVal(info["uuid"]),
		}

		// Mount options — can be a string array.
		if opts, ok := info["mount_options"]; ok {
			entry.MountOptions = toStringSlice(opts)
		}

		// Inode fields — optional, use pointers. Prefer the raw
		// inodes_percent_used value from Ohai; fall back to computing
		// from used/total if it cannot be parsed.
		if v, ok := info["inodes_used"]; ok {
			n := diskToInt64(v)
			if n >= 0 {
				entry.InodesUsed = &n
			}
		}
		var totalInodes int64
		if v, ok := info["total_inodes"]; ok {
			totalInodes = diskToInt64(v)
			if totalInodes >= 0 {
				entry.TotalInodes = &totalInodes
			}
		}
		if v, ok := info["inodes_available"]; ok {
			n := diskToInt64(v)
			if n >= 0 {
				entry.InodesAvailable = &n
			}
		}
		if v, ok := info["inodes_percent_used"]; ok {
			n := diskToInt64(v)
			if n > 0 {
				entry.InodesPercentUsed = &n
			}
		}
		if entry.InodesPercentUsed == nil && entry.InodesUsed != nil && totalInodes > 0 {
			pct := int64(math.Round(float64(*entry.InodesUsed) / float64(totalInodes) * 100))
			entry.InodesPercentUsed = &pct
		}

		// Windows-specific fields.
		if v, ok := info["drive_type_human"]; ok {
			entry.DriveType = stringVal(v)
		}
		if v, ok := info["volume_name"]; ok {
			entry.VolumeName = stringVal(v)
		}
		if v, ok := info["encryption_status"]; ok {
			entry.EncryptionStatus = stringVal(v)
		}

		entries = append(entries, entry)
	}

	// Sort by mount path for deterministic output.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Mount < entries[j].Mount
	})

	return entries, nil
}

// isVirtualFS returns true if the filesystem type or mount path indicates
// a virtual/pseudo filesystem that should be excluded by default.
func isVirtualFS(fsType, mount string) bool {
	if virtualFSTypes[fsType] {
		return true
	}
	// Filter common virtual mount paths.
	if strings.HasPrefix(mount, "/sys/") ||
		strings.HasPrefix(mount, "/proc/") ||
		strings.HasPrefix(mount, "/dev/") ||
		strings.HasPrefix(mount, "/run/") ||
		mount == "/dev" ||
		mount == "/proc" ||
		mount == "/sys" ||
		mount == "/run" {
		return true
	}
	return false
}

// filesystemEntriesByMount normalises either stored filesystem shape into a
// map of mount point → entry. Returns nil when the payload carries no usable
// filesystem data.
func filesystemEntriesByMount(raw json.RawMessage) (map[string]map[string]interface{}, error) {
	// Legacy full-subtree snapshot: sections nested under by_mountpoint /
	// by_pair / by_device. Detect the wrapper by any of those keys, not just
	// by_mountpoint — otherwise a wrapper missing that one section falls
	// through to the flat branch below and the section names themselves get
	// read as mount points.
	var wrapper struct {
		ByMountpoint map[string]map[string]interface{} `json:"by_mountpoint"`
		ByPair       map[string]map[string]interface{} `json:"by_pair"`
		ByDevice     map[string]map[string]interface{} `json:"by_device"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		switch {
		case wrapper.ByMountpoint != nil:
			// Keys are already mount points.
			return wrapper.ByMountpoint, nil
		case wrapper.ByPair != nil:
			return byMountFromPairs(wrapper.ByPair), nil
		case wrapper.ByDevice != nil:
			return byMountFromPairs(wrapper.ByDevice), nil
		}
	}

	// Narrowed snapshot: the by_pair map is stored directly, keyed
	// "device,mount".
	var pairs map[string]map[string]interface{}
	if err := json.Unmarshal(raw, &pairs); err != nil {
		return nil, fmt.Errorf("unmarshal filesystem: %w", err)
	}

	return byMountFromPairs(pairs), nil
}

// byMountFromPairs re-keys a "device,mount" map by mount point, dropping
// entries that have no mount.
func byMountFromPairs(pairs map[string]map[string]interface{}) map[string]map[string]interface{} {
	byMount := make(map[string]map[string]interface{}, len(pairs))
	for key, info := range pairs {
		mount := mountForPair(key, info)
		if mount == "" {
			// A pair with no mount half is a bare unmounted device — Ohai
			// emits a "/dev/sda," entry for every disk. Not a filesystem.
			continue
		}
		byMount[mount] = info
	}
	return byMount
}

// mountForPair resolves the mount point of a by_pair entry, preferring the
// explicit "mount" field and falling back to the mount half of the key.
//
// Some Ohai versions omit the redundant "mount" field from by_pair entries.
// analysis.EvaluateDisk tolerates that because findBestMountWindows matches
// the map key before falling back to entry.Mount, so such nodes report a disk
// verdict. Deriving from the key keeps this page consistent with that rather
// than requiring the field.
//
// The key is normally "<device>,<mount>" and a device name may itself contain
// a comma (e.g. "weird,device,/data"), hence the split on the LAST comma; an
// empty mount half means a bare unmounted device and yields "".
//
// Some Ohai versions (seen on Chef 16 Windows) instead key volumes by bare
// drive letter — "C:", "D:" — with no comma and no "mount" field. Such a key
// is accepted only when it actually looks like a mount point, so that stray
// top-level keys are never mistaken for one.
func mountForPair(key string, info map[string]interface{}) string {
	if mount := stringVal(info["mount"]); mount != "" {
		return mount
	}
	if i := strings.LastIndex(key, ","); i >= 0 {
		return key[i+1:]
	}
	if looksLikeMountPoint(key) {
		return key
	}
	return ""
}

// looksLikeMountPoint reports whether s has the shape of a mount point: a
// POSIX absolute path, a UNC path, or a Windows drive letter such as "C:".
func looksLikeMountPoint(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\\`) {
		return true
	}
	if len(s) >= 2 && s[1] == ':' {
		c := s[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}

// deviceFromInfo extracts the device name from the mountpoint info map.
// The "devices" field can be a string array; we return the first entry.
// Falls back to the "device" field if "devices" is absent.

func deviceFromInfo(info map[string]interface{}) string {
	if devs, ok := info["devices"]; ok {
		if sl := toStringSlice(devs); len(sl) > 0 {
			return sl[0]
		}
	}
	return stringVal(info["device"])
}

// diskToInt64 converts an interface{} value to int64, handling the fact
// that Ohai reports some numeric fields as strings on certain platforms.
func diskToInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		s = strings.TrimSuffix(s, "%")
		if s == "" {
			return 0
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			// Try parsing as float (some systems report "12345.0").
			f, fErr := strconv.ParseFloat(s, 64)
			if fErr != nil {
				return 0
			}
			return int64(math.Floor(f))
		}
		return n
	case float64:
		return int64(math.Floor(val))
	case float32:
		return int64(math.Floor(float64(val)))
	case int:
		return int64(val)
	case int64:
		return val
	case int32:
		return int64(val)
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			f, fErr := val.Float64()
			if fErr != nil {
				return 0
			}
			return int64(math.Floor(f))
		}
		return n
	default:
		return 0
	}
}

// stringVal safely extracts a string from an interface{} value.
func stringVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// toStringSlice converts an interface{} value to a []string. It handles
// both []interface{} (from JSON unmarshal into map[string]interface{}) and
// native []string values.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return val
	default:
		return nil
	}
}

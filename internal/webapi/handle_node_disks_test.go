// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// handleNodeDisks integration tests
// ---------------------------------------------------------------------------

func TestHandleNodeDisks_NotEnoughSegments(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleNodeDisks_MethodNotAllowed(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/disks/prod/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleNodeDisks_OrgNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/nope/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleNodeDisks_NodeNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/missing", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleNodeDisks_OrgDBError(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleNodeDisks_NodeDBError(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{}, errors.New("connection refused")
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/web1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleNodeDisks_HappyPath_Linux(t *testing.T) {
	fsData := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"devices": ["/dev/nvme0n1p1"],
				"fs_type": "ext4",
				"kb_size": "120984300",
				"kb_used": "22104852",
				"kb_available": "92687576",
				"percent_used": "20%",
				"uuid": "11e08d25-abcd-1234-efgh-567890abcdef",
				"mount_options": ["rw", "relatime"],
				"inodes_used": "234976",
				"total_inodes": "7725056",
				"inodes_available": "7490080",
				"inodes_percent_used": "4%"
			},
			"/boot": {
				"devices": ["/dev/nvme0n1p2"],
				"fs_type": "ext2",
				"kb_size": "524288",
				"kb_used": "102400",
				"kb_available": "421888",
				"percent_used": "20%"
			}
		}
	}`)
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "pandora.home.arpa",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     fsData,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/pandora.home.arpa", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.NodeName != "pandora.home.arpa" {
		t.Errorf("node_name = %q, want %q", body.NodeName, "pandora.home.arpa")
	}
	if body.OrganisationName != "prod" {
		t.Errorf("organisation_name = %q, want %q", body.OrganisationName, "prod")
	}
	if body.Platform != "ubuntu" {
		t.Errorf("platform = %q, want %q", body.Platform, "ubuntu")
	}
	if len(body.Disks) != 2 {
		t.Fatalf("disks count = %d, want 2", len(body.Disks))
	}

	// Sorted by mount: "/" before "/boot".
	if body.Disks[0].Mount != "/" {
		t.Errorf("disks[0].mount = %q, want %q", body.Disks[0].Mount, "/")
	}
	if body.Disks[0].Device != "/dev/nvme0n1p1" {
		t.Errorf("disks[0].device = %q, want %q", body.Disks[0].Device, "/dev/nvme0n1p1")
	}
	if body.Disks[0].FSType != "ext4" {
		t.Errorf("disks[0].fs_type = %q, want %q", body.Disks[0].FSType, "ext4")
	}
	if body.Disks[0].KBSize != 120984300 {
		t.Errorf("disks[0].kb_size = %d, want %d", body.Disks[0].KBSize, 120984300)
	}
	if body.Disks[0].KBUsed != 22104852 {
		t.Errorf("disks[0].kb_used = %d, want %d", body.Disks[0].KBUsed, 22104852)
	}
	if body.Disks[0].KBAvailable != 92687576 {
		t.Errorf("disks[0].kb_available = %d, want %d", body.Disks[0].KBAvailable, 92687576)
	}
	if body.Disks[0].PercentUsed != 20 {
		t.Errorf("disks[0].percent_used = %d, want %d", body.Disks[0].PercentUsed, 20)
	}
	if body.Disks[0].UUID != "11e08d25-abcd-1234-efgh-567890abcdef" {
		t.Errorf("disks[0].uuid = %q, want %q", body.Disks[0].UUID, "11e08d25-abcd-1234-efgh-567890abcdef")
	}
	if len(body.Disks[0].MountOptions) != 2 || body.Disks[0].MountOptions[0] != "rw" {
		t.Errorf("disks[0].mount_options = %v, want [rw relatime]", body.Disks[0].MountOptions)
	}
	if body.Disks[0].InodesUsed == nil || *body.Disks[0].InodesUsed != 234976 {
		t.Errorf("disks[0].inodes_used = %v, want 234976", body.Disks[0].InodesUsed)
	}
	if body.Disks[0].TotalInodes == nil || *body.Disks[0].TotalInodes != 7725056 {
		t.Errorf("disks[0].total_inodes = %v, want 7725056", body.Disks[0].TotalInodes)
	}
	if body.Disks[0].InodesAvailable == nil || *body.Disks[0].InodesAvailable != 7490080 {
		t.Errorf("disks[0].inodes_available = %v, want 7490080", body.Disks[0].InodesAvailable)
	}
	if body.Disks[0].InodesPercentUsed == nil || *body.Disks[0].InodesPercentUsed != 4 {
		t.Errorf("disks[0].inodes_percent_used = %v, want 4", body.Disks[0].InodesPercentUsed)
	}

	if body.Disks[1].Mount != "/boot" {
		t.Errorf("disks[1].mount = %q, want %q", body.Disks[1].Mount, "/boot")
	}
	// /boot should not have inode fields set.
	if body.Disks[1].InodesPercentUsed != nil {
		t.Errorf("disks[1].inodes_percent_used = %v, want nil", body.Disks[1].InodesPercentUsed)
	}
	if body.Disks[1].InodesUsed != nil {
		t.Errorf("disks[1].inodes_used = %v, want nil", body.Disks[1].InodesUsed)
	}
}

func TestHandleNodeDisks_HappyPath_Windows(t *testing.T) {
	fsData := json.RawMessage(`{
		"by_mountpoint": {
			"C:": {
				"fs_type": "ntfs",
				"kb_size": 41949327,
				"kb_used": 36166840,
				"kb_available": 5782487,
				"percent_used": 86,
				"drive_type_human": "Local Fixed Disk",
				"volume_name": "",
				"encryption_status": "FullyEncrypted"
			},
			"D:": {
				"fs_type": "ntfs",
				"kb_size": 102400000,
				"kb_used": 51200000,
				"kb_available": 51200000,
				"percent_used": 50,
				"drive_type_human": "Local Fixed Disk",
				"volume_name": "Data"
			}
		}
	}`)
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "win11-001.home.arpa",
				OrganisationName: "org-1",
				Platform:       "windows",
				Filesystem:     fsData,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/win11-001.home.arpa", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Platform != "windows" {
		t.Errorf("platform = %q, want %q", body.Platform, "windows")
	}
	if len(body.Disks) != 2 {
		t.Fatalf("disks count = %d, want 2", len(body.Disks))
	}

	// Sorted: "C:" before "D:".
	if body.Disks[0].Mount != "C:" {
		t.Errorf("disks[0].mount = %q, want %q", body.Disks[0].Mount, "C:")
	}
	if body.Disks[0].KBSize != 41949327 {
		t.Errorf("disks[0].kb_size = %d, want %d", body.Disks[0].KBSize, 41949327)
	}
	if body.Disks[0].DriveType != "Local Fixed Disk" {
		t.Errorf("disks[0].drive_type = %q, want %q", body.Disks[0].DriveType, "Local Fixed Disk")
	}
	if body.Disks[0].EncryptionStatus != "FullyEncrypted" {
		t.Errorf("disks[0].encryption_status = %q, want %q", body.Disks[0].EncryptionStatus, "FullyEncrypted")
	}

	if body.Disks[1].Mount != "D:" {
		t.Errorf("disks[1].mount = %q, want %q", body.Disks[1].Mount, "D:")
	}
	if body.Disks[1].VolumeName != "Data" {
		t.Errorf("disks[1].volume_name = %q, want %q", body.Disks[1].VolumeName, "Data")
	}
}

func TestHandleNodeDisks_NullFilesystem(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "empty-node",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     nil,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/empty-node", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Disks) != 0 {
		t.Errorf("disks count = %d, want 0", len(body.Disks))
	}
}

func TestHandleNodeDisks_FiltersVirtualFS(t *testing.T) {
	fsData := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"devices": ["/dev/sda1"],
				"fs_type": "ext4",
				"kb_size": 100000,
				"kb_used": 50000,
				"kb_available": 50000,
				"percent_used": 50
			},
			"/proc": {
				"devices": ["proc"],
				"fs_type": "proc",
				"kb_size": 0,
				"kb_used": 0,
				"kb_available": 0,
				"percent_used": 0
			},
			"/sys": {
				"devices": ["sysfs"],
				"fs_type": "sysfs",
				"kb_size": 0,
				"kb_used": 0,
				"kb_available": 0,
				"percent_used": 0
			},
			"/dev/pts": {
				"devices": ["devpts"],
				"fs_type": "devpts",
				"kb_size": 0,
				"kb_used": 0,
				"kb_available": 0,
				"percent_used": 0
			},
			"/dev/shm": {
				"devices": ["tmpfs"],
				"fs_type": "tmpfs",
				"kb_size": 1024,
				"kb_used": 0,
				"kb_available": 1024,
				"percent_used": 0
			},
			"/snap/core/12345": {
				"devices": ["/dev/loop0"],
				"fs_type": "squashfs",
				"kb_size": 65536,
				"kb_used": 65536,
				"kb_available": 0,
				"percent_used": 100
			},
			"/sys/fs/cgroup": {
				"devices": ["cgroup2"],
				"fs_type": "cgroup2",
				"kb_size": 0,
				"kb_used": 0,
				"kb_available": 0,
				"percent_used": 0
			},
			"/run/user/1000": {
				"devices": ["tmpfs"],
				"fs_type": "tmpfs",
				"kb_size": 1024,
				"kb_used": 0,
				"kb_available": 1024,
				"percent_used": 0
			}
		}
	}`)
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "linux-host",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     fsData,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/linux-host", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Only "/" should remain after filtering.
	if len(body.Disks) != 1 {
		t.Fatalf("disks count = %d, want 1; got: %+v", len(body.Disks), body.Disks)
	}
	if body.Disks[0].Mount != "/" {
		t.Errorf("disks[0].mount = %q, want %q", body.Disks[0].Mount, "/")
	}
}

func TestHandleNodeDisks_ShowAllIncludesVirtualFS(t *testing.T) {
	fsData := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"devices": ["/dev/sda1"],
				"fs_type": "ext4",
				"kb_size": 100000,
				"kb_used": 50000,
				"kb_available": 50000,
				"percent_used": 50
			},
			"/proc": {
				"devices": ["proc"],
				"fs_type": "proc",
				"kb_size": 0,
				"kb_used": 0,
				"kb_available": 0,
				"percent_used": 0
			},
			"/sys": {
				"devices": ["sysfs"],
				"fs_type": "sysfs",
				"kb_size": 0,
				"kb_used": 0,
				"kb_available": 0,
				"percent_used": 0
			}
		}
	}`)
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "linux-host",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     fsData,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/linux-host?show_all=true", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Disks) != 3 {
		t.Errorf("disks count = %d, want 3 (show_all=true)", len(body.Disks))
	}
}

func TestHandleNodeDisks_NodeNameWithSlash(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	calledWith := ""
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			calledWith = nodeName
			return datastore.NodeSnapshot{
				NodeName:       nodeName,
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     nil,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/some/node/with/slashes", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if calledWith != "some/node/with/slashes" {
		t.Errorf("node name = %q, want %q", calledWith, "some/node/with/slashes")
	}
}

func TestHandleNodeDisks_MalformedJSON(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "bad-json",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     json.RawMessage(`{not valid json`),
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/bad-json", nil)
	r.ServeHTTP(w, req)

	// Should return 200 with empty disks, not 500.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Disks) != 0 {
		t.Errorf("disks count = %d, want 0", len(body.Disks))
	}
}

func TestHandleNodeDisks_EmptyByMountpoint(t *testing.T) {
	fsData := json.RawMessage(`{
		"by_pair": {},
		"by_device": {},
		"by_mountpoint": {}
	}`)
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "empty-mounts",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     fsData,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/empty-mounts", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Disks) != 0 {
		t.Errorf("disks count = %d, want 0", len(body.Disks))
	}
}

func TestHandleNodeDisks_NoByMountpointKey(t *testing.T) {
	fsData := json.RawMessage(`{
		"by_pair": {"something": {}},
		"by_device": {"something": {}}
	}`)
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{Name: "prod"}, nil
		},
		GetNodeSnapshotByNameFn: func(ctx context.Context, orgID, nodeName string) (datastore.NodeSnapshot, error) {
			return datastore.NodeSnapshot{
				NodeName:       "no-mount-key",
				OrganisationName: "org-1",
				Platform:       "ubuntu",
				Filesystem:     fsData,
				CollectedAt:    now,
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/disks/prod/no-mount-key", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body diskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Disks) != 0 {
		t.Errorf("disks count = %d, want 0", len(body.Disks))
	}
}

// ---------------------------------------------------------------------------
// parseFilesystemData unit tests
// ---------------------------------------------------------------------------

func TestParseFilesystemData_NilInput(t *testing.T) {
	disks, err := parseFilesystemData(nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 0 {
		t.Errorf("disks count = %d, want 0", len(disks))
	}
}

func TestParseFilesystemData_NullString(t *testing.T) {
	disks, err := parseFilesystemData(json.RawMessage(`null`), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 0 {
		t.Errorf("disks count = %d, want 0", len(disks))
	}
}

func TestParseFilesystemData_SortedByMount(t *testing.T) {
	raw := json.RawMessage(`{
		"by_mountpoint": {
			"/var": {"fs_type": "ext4", "kb_size": 1000, "kb_used": 100, "kb_available": 900, "percent_used": 10},
			"/": {"fs_type": "ext4", "kb_size": 2000, "kb_used": 200, "kb_available": 1800, "percent_used": 10},
			"/home": {"fs_type": "ext4", "kb_size": 3000, "kb_used": 300, "kb_available": 2700, "percent_used": 10}
		}
	}`)
	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 3 {
		t.Fatalf("disks count = %d, want 3", len(disks))
	}
	expected := []string{"/", "/home", "/var"}
	for i, want := range expected {
		if disks[i].Mount != want {
			t.Errorf("disks[%d].mount = %q, want %q", i, disks[i].Mount, want)
		}
	}
}

func TestParseFilesystemData_NumericValuesAsIntegers(t *testing.T) {
	raw := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"fs_type": "ext4",
				"kb_size": 120984300,
				"kb_used": 22104852,
				"kb_available": 92687576,
				"percent_used": 20
			}
		}
	}`)
	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("disks count = %d, want 1", len(disks))
	}
	if disks[0].KBSize != 120984300 {
		t.Errorf("kb_size = %d, want 120984300", disks[0].KBSize)
	}
	if disks[0].KBUsed != 22104852 {
		t.Errorf("kb_used = %d, want 22104852", disks[0].KBUsed)
	}
}

func TestParseFilesystemData_NumericValuesAsStrings(t *testing.T) {
	raw := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"fs_type": "ext4",
				"kb_size": "120984300",
				"kb_used": "22104852",
				"kb_available": "92687576",
				"percent_used": "20"
			}
		}
	}`)
	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("disks count = %d, want 1", len(disks))
	}
	if disks[0].KBSize != 120984300 {
		t.Errorf("kb_size = %d, want 120984300", disks[0].KBSize)
	}
}

func TestParseFilesystemData_DeviceFallback(t *testing.T) {
	raw := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"device": "/dev/fallback",
				"fs_type": "ext4",
				"kb_size": 1000,
				"kb_used": 100,
				"kb_available": 900,
				"percent_used": 10
			}
		}
	}`)
	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("disks count = %d, want 1", len(disks))
	}
	if disks[0].Device != "/dev/fallback" {
		t.Errorf("device = %q, want %q", disks[0].Device, "/dev/fallback")
	}
}

func TestParseFilesystemData_DevicesArrayPrefersFirst(t *testing.T) {
	raw := json.RawMessage(`{
		"by_mountpoint": {
			"/": {
				"devices": ["/dev/sda1", "/dev/sdb1"],
				"fs_type": "ext4",
				"kb_size": 1000,
				"kb_used": 100,
				"kb_available": 900,
				"percent_used": 10
			}
		}
	}`)
	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("disks count = %d, want 1", len(disks))
	}
	if disks[0].Device != "/dev/sda1" {
		t.Errorf("device = %q, want %q", disks[0].Device, "/dev/sda1")
	}
}

// ---------------------------------------------------------------------------
// isVirtualFS unit tests
// ---------------------------------------------------------------------------

func TestIsVirtualFS_KnownTypes(t *testing.T) {
	cases := []struct {
		fsType string
		mount  string
		want   bool
	}{
		{"proc", "/proc", true},
		{"sysfs", "/sys", true},
		{"devpts", "/dev/pts", true},
		{"tmpfs", "/dev/shm", true},
		{"squashfs", "/snap/core/12345", true},
		{"cgroup2", "/sys/fs/cgroup", true},
		{"bpf", "/sys/fs/bpf", true},
		{"ext4", "/", false},
		{"xfs", "/data", false},
		{"ntfs", "C:", false},
		// Virtual by mount path even if fs_type is unrecognised.
		{"unknown", "/proc/something", true},
		{"unknown", "/sys/kernel", true},
		{"unknown", "/dev/mqueue", true},
		{"unknown", "/run/lock", true},
	}
	for _, tc := range cases {
		got := isVirtualFS(tc.fsType, tc.mount)
		if got != tc.want {
			t.Errorf("isVirtualFS(%q, %q) = %v, want %v", tc.fsType, tc.mount, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// diskToInt64 unit tests
// ---------------------------------------------------------------------------

func TestDiskToInt64(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want int64
	}{
		{"nil", nil, 0},
		{"string int", "12345", 12345},
		{"string float", "12345.7", 12345},
		{"string percent", "20%", 20},
		{"string percent with space", " 4% ", 4},
		{"string empty", "", 0},
		{"string garbage", "abc", 0},
		{"float64", float64(99999), 99999},
		{"float64 frac", float64(99999.9), 99999},
		{"int", int(42), 42},
		{"int64", int64(999), 999},
	}
	for _, tc := range cases {
		got := diskToInt64(tc.in)
		if got != tc.want {
			t.Errorf("diskToInt64(%v [%s]) = %d, want %d", tc.in, tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// stringVal unit tests
// ---------------------------------------------------------------------------

func TestStringVal(t *testing.T) {
	if got := stringVal(nil); got != "" {
		t.Errorf("stringVal(nil) = %q, want %q", got, "")
	}
	if got := stringVal("hello"); got != "hello" {
		t.Errorf("stringVal(\"hello\") = %q, want %q", got, "hello")
	}
	if got := stringVal(42); got != "42" {
		t.Errorf("stringVal(42) = %q, want %q", got, "42")
	}
}

// ---------------------------------------------------------------------------
// toStringSlice unit tests
// ---------------------------------------------------------------------------

func TestToStringSlice(t *testing.T) {
	if got := toStringSlice(nil); got != nil {
		t.Errorf("toStringSlice(nil) = %v, want nil", got)
	}

	in := []interface{}{"rw", "relatime", "noatime"}
	got := toStringSlice(in)
	if len(got) != 3 || got[0] != "rw" || got[1] != "relatime" || got[2] != "noatime" {
		t.Errorf("toStringSlice([]interface{}) = %v, want [rw relatime noatime]", got)
	}

	native := []string{"rw", "nosuid"}
	got2 := toStringSlice(native)
	if len(got2) != 2 || got2[0] != "rw" || got2[1] != "nosuid" {
		t.Errorf("toStringSlice([]string) = %v, want [rw nosuid]", got2)
	}

	if got3 := toStringSlice(12345); got3 != nil {
		t.Errorf("toStringSlice(int) = %v, want nil", got3)
	}
}

// ---------------------------------------------------------------------------
// deviceFromInfo unit tests
// ---------------------------------------------------------------------------

func TestDeviceFromInfo_Devices(t *testing.T) {
	info := map[string]interface{}{
		"devices": []interface{}{"/dev/sda1"},
	}
	if got := deviceFromInfo(info); got != "/dev/sda1" {
		t.Errorf("deviceFromInfo(devices) = %q, want %q", got, "/dev/sda1")
	}
}

func TestDeviceFromInfo_DeviceFallback(t *testing.T) {
	info := map[string]interface{}{
		"device": "/dev/fallback",
	}
	if got := deviceFromInfo(info); got != "/dev/fallback" {
		t.Errorf("deviceFromInfo(device) = %q, want %q", got, "/dev/fallback")
	}
}

func TestDeviceFromInfo_Neither(t *testing.T) {
	info := map[string]interface{}{
		"fs_type": "ext4",
	}
	if got := deviceFromInfo(info); got != "" {
		t.Errorf("deviceFromInfo(empty) = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// by_pair payloads (the shape the node partial search now stores)
// ---------------------------------------------------------------------------
//
// The node search requests ["filesystem","by_pair"], so node_snapshots.filesystem
// holds the by_pair map directly — keyed "device,mount" with no by_mountpoint
// wrapper. parseFilesystemData must read that shape, and must keep reading the
// legacy by_mountpoint wrapper for snapshots stored before the change (both
// coexist until every org has been re-collected).

func TestParseFilesystemData_ByPairLinux(t *testing.T) {
	// Verbatim from node_snapshots.filesystem for a real Ubuntu 24.04 node.
	raw := json.RawMessage(`{
		"/dev/mapper/vg-lv,/": {
			"uuid": "2a0e5534-2c57-464f-ae3b-f2838143207d", "mount": "/",
			"device": "/dev/mapper/vg-lv", "fs_type": "ext4",
			"kb_size": "24590672", "kb_used": "7468364", "kb_available": "15847840",
			"percent_used": "33%", "inodes_used": "147193", "total_inodes": "1572864",
			"inodes_available": "1425671", "inodes_percent_used": "10%",
			"mount_options": ["rw", "relatime"]
		},
		"/dev/sda2,/boot": {
			"mount": "/boot", "device": "/dev/sda2", "fs_type": "ext4",
			"kb_size": "1992552", "kb_used": "200000", "kb_available": "1700000",
			"percent_used": "11%"
		},
		"/dev/sda,": {"device": "/dev/sda", "fs_type": "ext4"}
	}`)

	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("expected 2 mounted filesystems, got %d: %+v", len(disks), disks)
	}

	// Sorted by mount: "/" then "/boot".
	root := disks[0]
	if root.Mount != "/" {
		t.Errorf("Mount = %q, want %q", root.Mount, "/")
	}
	if root.Device != "/dev/mapper/vg-lv" {
		t.Errorf("Device = %q, want the by_pair 'device' string", root.Device)
	}
	if root.FSType != "ext4" {
		t.Errorf("FSType = %q, want ext4", root.FSType)
	}
	if root.KBAvailable != 15847840 {
		t.Errorf("KBAvailable = %d, want 15847840", root.KBAvailable)
	}
	if root.PercentUsed != 33 {
		t.Errorf("PercentUsed = %d, want 33 (Ohai reports '33%%')", root.PercentUsed)
	}
	if root.UUID != "2a0e5534-2c57-464f-ae3b-f2838143207d" {
		t.Errorf("UUID = %q, want the by_pair uuid", root.UUID)
	}
	if len(root.MountOptions) != 2 {
		t.Errorf("MountOptions = %v, want 2 entries", root.MountOptions)
	}
	if root.InodesPercentUsed == nil || *root.InodesPercentUsed != 10 {
		t.Errorf("InodesPercentUsed = %v, want 10", root.InodesPercentUsed)
	}
}

func TestParseFilesystemData_ByPairSkipsUnmountedDevices(t *testing.T) {
	// by_pair includes bare devices with an empty mount half ("/dev/sda,").
	// They have no mount point and must not appear as blank-mount rows.
	raw := json.RawMessage(`{
		"/dev/sda,":  {"device": "/dev/sda", "fs_type": "ext4"},
		"/dev/sr0,":  {"device": "/dev/sr0"},
		"/dev/sda1,/": {"mount": "/", "device": "/dev/sda1", "fs_type": "ext4", "kb_size": "1000", "kb_available": "500"}
	}`)

	disks, err := parseFilesystemData(raw, true)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected only the mounted entry, got %d: %+v", len(disks), disks)
	}
	if disks[0].Mount != "/" {
		t.Errorf("Mount = %q, want %q", disks[0].Mount, "/")
	}
}

func TestParseFilesystemData_ByPairWindows(t *testing.T) {
	// Verbatim from node_snapshots.filesystem for a real Windows Server 2022
	// node. Keys are ",C:" — the device half is empty, so the mount must come
	// from the entry's "mount" field, not the key.
	raw := json.RawMessage(`{
		",C:": {"mount": "C:", "device": "", "fs_type": "ntfs", "kb_size": 33685499,
			"kb_used": 15392510, "kb_available": 18292989, "percent_used": 45,
			"drive_type_human": "Local Fixed Disk", "volume_name": ""},
		"new volume,D:": {"mount": "D:", "device": "new volume", "fs_type": "ntfs",
			"kb_size": 34340859, "kb_used": 1403424, "kb_available": 32937435,
			"percent_used": 4, "drive_type_human": "Local Fixed Disk", "volume_name": "New Volume"}
	}`)

	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("expected 2 Windows volumes, got %d: %+v", len(disks), disks)
	}
	if disks[0].Mount != "C:" {
		t.Errorf("Mount = %q, want C:", disks[0].Mount)
	}
	if disks[0].KBAvailable != 18292989 {
		t.Errorf("KBAvailable = %d, want 18292989", disks[0].KBAvailable)
	}
	if disks[0].DriveType != "Local Fixed Disk" {
		t.Errorf("DriveType = %q, want %q", disks[0].DriveType, "Local Fixed Disk")
	}
	if disks[1].VolumeName != "New Volume" {
		t.Errorf("VolumeName = %q, want %q", disks[1].VolumeName, "New Volume")
	}
}

func TestParseFilesystemData_ByMountpointStillSupported(t *testing.T) {
	// Snapshots stored before the search was narrowed still carry the full
	// wrapper; they must keep rendering until those orgs are re-collected.
	raw := json.RawMessage(`{
		"by_mountpoint": {
			"/": {"devices": ["/dev/sda1"], "fs_type": "ext4", "kb_size": "1000", "kb_available": "500", "percent_used": "50%"}
		},
		"by_device": {"/dev/sda1": {"mount": "/"}}
	}`)

	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected 1 filesystem, got %d", len(disks))
	}
	if disks[0].Mount != "/" || disks[0].Device != "/dev/sda1" {
		t.Errorf("got mount=%q device=%q, want / and /dev/sda1", disks[0].Mount, disks[0].Device)
	}
}

func TestParseFilesystemData_ByPairDerivesMountFromKeyWhenFieldAbsent(t *testing.T) {
	// by_pair keys encode "device,mount". Some Ohai versions omit the
	// redundant "mount" field from the entry. EvaluateDisk still resolves
	// those because findBestMountWindows matches the key before falling back
	// to entry.Mount — so a disk verdict appears while the disks page renders
	// nothing. Derive the mount from the key so both agree.
	raw := json.RawMessage(`{
		",C:":            {"fs_type": "ntfs", "kb_size": 33685499, "kb_used": 15392510, "kb_available": 18292989},
		"new volume,D:":  {"fs_type": "ntfs", "kb_size": 34340859, "kb_used": 1403424, "kb_available": 32937435},
		"/dev/sda1,/":    {"fs_type": "ext4", "kb_size": 1000, "kb_used": 400, "kb_available": 600},
		"/dev/sda,":      {"fs_type": "ext4"}
	}`)

	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 3 {
		t.Fatalf("expected 3 mounted filesystems, got %d: %+v", len(disks), disks)
	}

	got := map[string]int64{}
	for _, d := range disks {
		got[d.Mount] = d.KBAvailable
	}
	for mount, want := range map[string]int64{"C:": 18292989, "D:": 32937435, "/": 600} {
		if got[mount] != want {
			t.Errorf("mount %q: KBAvailable = %d, want %d (all mounts: %v)", mount, got[mount], want, got)
		}
	}
	// "/dev/sda," has an empty mount half — an unmounted device, not a filesystem.
	if _, ok := got[""]; ok {
		t.Error("entry with an empty mount half must be skipped, not rendered blank")
	}
}

func TestParseFilesystemData_ByPairPrefersMountFieldOverKey(t *testing.T) {
	// When both are present the explicit field wins — the key is only a
	// fallback, and a device name may itself contain a comma.
	raw := json.RawMessage(`{
		"weird,device,/data": {"mount": "/data", "fs_type": "xfs", "kb_size": 100, "kb_available": 50}
	}`)

	disks, err := parseFilesystemData(raw, true)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected 1 filesystem, got %d", len(disks))
	}
	if disks[0].Mount != "/data" {
		t.Errorf("Mount = %q, want /data", disks[0].Mount)
	}
}

func TestParseFilesystemData_ByPairWindowsDriveLetterKeys(t *testing.T) {
	// Verbatim shape from a customer Windows node (Ohai version differs from
	// the lab): by_pair keys are bare drive letters with NO comma and the
	// entries carry NO "mount" field.
	//
	// analysis.findBestMountWindows matches the key against the drive letter,
	// so these nodes report a disk verdict — while this page rendered nothing
	// because it required either a "mount" field or a "device,mount" key.
	raw := json.RawMessage(`{
		"C:": {"fs_type": "ntfs", "kb_size": 98782146, "kb_used": 55134618, "drive_type": 3,
			"volume_name": "System", "kb_available": 43647528, "percent_used": 55,
			"drive_type_human": "Local Fixed Disk", "drive_type_string": "local"},
		"D:": {"fs_type": "ntfs", "kb_size": 127775272, "kb_used": 102952689,
			"kb_available": 24822583, "percent_used": 81, "drive_type_human": "Local Fixed Disk"}
	}`)

	disks, err := parseFilesystemData(raw, false)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("expected 2 Windows volumes, got %d: %+v", len(disks), disks)
	}
	if disks[0].Mount != "C:" || disks[1].Mount != "D:" {
		t.Fatalf("mounts = %q/%q, want C:/D:", disks[0].Mount, disks[1].Mount)
	}
	if disks[0].KBAvailable != 43647528 {
		t.Errorf("C: KBAvailable = %d, want 43647528", disks[0].KBAvailable)
	}
	if disks[0].VolumeName != "System" || disks[0].DriveType != "Local Fixed Disk" {
		t.Errorf("C: volume=%q driveType=%q, want System / Local Fixed Disk",
			disks[0].VolumeName, disks[0].DriveType)
	}
}

func TestParseFilesystemData_CommalessNonMountKeysIgnored(t *testing.T) {
	// A comma-less key is only a mount if it looks like one. Section names
	// from a partially-populated wrapper must never become mount points.
	raw := json.RawMessage(`{"by_pair": {"something": {}}, "by_device": {"something": {}}}`)

	disks, err := parseFilesystemData(raw, true)
	if err != nil {
		t.Fatalf("parseFilesystemData: %v", err)
	}
	if len(disks) != 0 {
		t.Errorf("expected 0 filesystems, got %d: %+v", len(disks), disks)
	}
}

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func (r *Router) handleOrganisations(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations: %v", err)
		WriteInternalError(w, "Failed to list organisations.")
		return
	}
	type orgResp struct {
		Name                 string `json:"name"`
		ChefServerURL        string `json:"chef_server_url"`
		OrgName              string `json:"org_name"`
		ClientName           string `json:"client_name"`
		CredentialSource     string `json:"credential_source"`
		Source               string `json:"source"`
		NodeCount            int    `json:"node_count"`
		LastCollectedAt      string `json:"last_collected_at,omitempty"`
		LastCollectionStatus string `json:"last_collection_status,omitempty"`
	}
	result := make([]orgResp, 0, len(orgs))
	for _, org := range orgs {
		item := orgResp{
			Name:             org.Name,
			ChefServerURL:    org.ChefServerURL,
			OrgName:          org.OrgName,
			ClientName:       org.ClientName,
			Source:           org.Source,
			CredentialSource: "config",
		}
		if org.ClientKeyCredentialID != "" {
			item.CredentialSource = "secrets_store"
		}
		latest, latestErr := r.db.GetLatestCollectionRun(req.Context(), org.ID)
		if latestErr == nil {
			item.LastCollectionStatus = latest.Status
			if !latest.CompletedAt.IsZero() {
				item.LastCollectedAt = latest.CompletedAt.Format("2006-01-02T15:04:05Z")
			} else {
				item.LastCollectedAt = latest.StartedAt.Format("2006-01-02T15:04:05Z")
			}
			item.NodeCount = latest.NodesCollected
		} else if !errors.Is(latestErr, datastore.ErrNotFound) {
			r.logf("WARN", "getting latest run for org %s: %v", org.Name, latestErr)
		}
		result = append(result, item)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (r *Router) handleOrganisationDetail(w http.ResponseWriter, req *http.Request) {
	segs := pathSegments(req.URL.Path, "/api/v1/organisations/")
	if len(segs) == 0 {
		WriteNotFound(w, "Organisation name is required.")
		return
	}
	orgName := segs[0]
	if len(segs) == 2 && segs[1] == "test" {
		WriteError(w, http.StatusNotImplemented, "not_implemented",
			fmt.Sprintf("Endpoint %s %s is not yet implemented.", req.Method, req.URL.Path))
		return
	}
	if !requireGET(w, req) {
		return
	}
	org, err := r.db.GetOrganisationByName(req.Context(), orgName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting organisation %s: %v", orgName, err)
		WriteInternalError(w, "Failed to get organisation.")
		return
	}
	WriteJSON(w, http.StatusOK, org)
}

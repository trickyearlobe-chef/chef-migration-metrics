// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDeleteKitchenBatch_DeletableStatuses verifies that batches in a terminal
// status (completed, cancelled, failed) and the pre-run draft status can be
// deleted, while in-flight batches (preparing, previewing, running) cannot.
//
// "failed" is a terminal status set by failBatch, so it must be in the
// deletable set; leaving it out makes Delete return "not found or not in a
// deletable status".
func TestDeleteKitchenBatch_DeletableStatuses(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	cleanupTestData(t, db, "DELETE FROM kitchen_batches WHERE name LIKE 'test-delete-batch-%'")

	deletable := []string{
		BatchStatusDraft,
		BatchStatusCompleted,
		BatchStatusCancelled,
		BatchStatusFailed,
	}
	notDeletable := []string{
		BatchStatusPreparing,
		BatchStatusPreviewing,
		BatchStatusRunning,
	}

	for _, status := range deletable {
		t.Run("deletable/"+status, func(t *testing.T) {
			b, err := db.CreateKitchenBatch(ctx, CreateKitchenBatchParams{
				Name: "test-delete-batch-" + status,
			})
			if err != nil {
				t.Fatalf("creating batch: %v", err)
			}
			if _, err := db.UpdateKitchenBatchStatus(ctx, b.ID, status, time.Now().UTC()); err != nil {
				t.Fatalf("setting status %q: %v", status, err)
			}
			if err := db.DeleteKitchenBatch(ctx, b.ID); err != nil {
				t.Fatalf("deleting %q batch: want nil, got %v", status, err)
			}
			if _, err := db.GetKitchenBatch(ctx, b.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("batch should be gone after delete: got err %v", err)
			}
		})
	}

	for _, status := range notDeletable {
		t.Run("not-deletable/"+status, func(t *testing.T) {
			b, err := db.CreateKitchenBatch(ctx, CreateKitchenBatchParams{
				Name: "test-delete-batch-" + status,
			})
			if err != nil {
				t.Fatalf("creating batch: %v", err)
			}
			if _, err := db.UpdateKitchenBatchStatus(ctx, b.ID, status, time.Now().UTC()); err != nil {
				t.Fatalf("setting status %q: %v", status, err)
			}
			if err := db.DeleteKitchenBatch(ctx, b.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("deleting in-flight %q batch: want ErrNotFound, got %v", status, err)
			}
		})
	}
}

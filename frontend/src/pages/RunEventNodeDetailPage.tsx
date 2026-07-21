// SPDX-License-Identifier: Apache-2.0

import { useParams, Link } from "react-router-dom";
import { NodeRunsSection } from "../components/NodeRunsSection";
import { fetchRunEventNodeRuns } from "../api";

// RunEventNodeDetailPage shows every converge run for one node (its full
// converge_runs history), reachable from the Run events Nodes tab. It keys on
// the delivered org name via fetchRunEventNodeRuns, so ingest-only DMZ nodes
// (no organisations row, no node_snapshots — hence no standard Node Detail page)
// resolve here.
export function RunEventNodeDetailPage() {
  const params = useParams<{ organisation: string; node: string }>();
  const organisation = params.organisation ?? "";
  const nodeName = params.node ?? "";

  return (
    <div className="space-y-4">
      <div>
        <Link
          to="/run-events"
          className="text-sm text-blue-600 hover:underline"
        >
          ← Back to run events
        </Link>
      </div>

      <div>
        <h2 className="text-xl font-bold text-gray-800">{nodeName}</h2>
        <p className="text-sm text-gray-500">
          Organisation <span className="font-mono">{organisation}</span> · ingest
          telemetry
        </p>
      </div>

      <NodeRunsSection
        org={organisation}
        nodeName={nodeName}
        fetchRuns={fetchRunEventNodeRuns}
      />
    </div>
  );
}

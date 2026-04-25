// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

interface PlatformLabelProps {
  platform?: string;
  platformVersion?: string;
  platformDisplayName?: string | null;
}

export function PlatformLabel({
  platform,
  platformVersion,
  platformDisplayName,
}: PlatformLabelProps) {
  if (!platform) return <span>—</span>;

  const rawLabel = platformVersion ? `${platform} ${platformVersion}` : platform;

  if (platformDisplayName) {
    return (
      <span title={rawLabel} className="cursor-help border-b border-dotted border-gray-400">
        {platformDisplayName}
      </span>
    );
  }

  return <span>{rawLabel}</span>;
}

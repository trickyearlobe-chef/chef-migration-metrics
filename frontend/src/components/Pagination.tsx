import type { Pagination as PaginationType } from "../types";

interface PaginationProps {
  pagination: PaginationType;
  onPageChange: (page: number) => void;
}

/**
 * Pagination controls for list pages. Shows page numbers with prev/next
 * buttons and a summary of total items.
 *
 * Matches the backend pagination envelope:
 *   { page, per_page, total_items, total_pages }
 */
export function Pagination({ pagination, onPageChange }: PaginationProps) {
  const { page, total_items, total_pages } = pagination;

  if (total_pages <= 1) {
    return (
      <div className="flex items-center justify-between px-1 py-3 text-sm text-gray-500">
        <span>
          {total_items} {total_items === 1 ? "item" : "items"}
        </span>
      </div>
    );
  }

  // Build an array of page numbers to show, with ellipses for gaps.
  // Always show first, last, and up to 2 pages around the current page.
  const pages = buildPageNumbers(page, total_pages);

  const start = (page - 1) * pagination.per_page + 1;
  const end = Math.min(page * pagination.per_page, total_items);

  return (
    <div className="flex flex-col items-center justify-between gap-3 px-1 py-3 sm:flex-row">
      {/* Item range summary */}
      <span className="text-sm text-gray-500">
        Showing {start}–{end} of {total_items.toLocaleString()}{" "}
        {total_items === 1 ? "item" : "items"}
      </span>

      {/* Page buttons */}
      <nav className="flex items-center gap-1" aria-label="Pagination">
        {/* Previous */}
        <button
          className="pagination-btn"
          onClick={() => onPageChange(page - 1)}
          disabled={page <= 1}
          aria-label="Previous page"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M15.75 19.5 8.25 12l7.5-7.5"
            />
          </svg>
        </button>

        {pages.map((p, i) =>
          p === null ? (
            <span
              key={`ellipsis-${i}`}
              className="px-1 text-sm text-gray-400 select-none"
              aria-hidden="true"
            >
              …
            </span>
          ) : (
            <button
              key={p}
              className={`pagination-btn ${p === page ? "active" : ""}`}
              onClick={() => onPageChange(p)}
              aria-current={p === page ? "page" : undefined}
              aria-label={`Page ${p}`}
            >
              {p}
            </button>
          ),
        )}

        {/* Next */}
        <button
          className="pagination-btn"
          onClick={() => onPageChange(page + 1)}
          disabled={page >= total_pages}
          aria-label="Next page"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="m8.25 4.5 7.5 7.5-7.5 7.5"
            />
          </svg>
        </button>
      </nav>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Builds an array of page numbers (and null for ellipses) to render.
 *
 * Strategy:
 *  - Always show page 1 and the last page.
 *  - Show up to `siblings` pages either side of the current page.
 *  - Insert null (ellipsis) where there's a gap > 1.
 */
function buildPageNumbers(
  current: number,
  total: number,
  siblings = 1,
): (number | null)[] {
  // For small totals, show all pages.
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }

  const pages: (number | null)[] = [];

  // Determine the range around the current page.
  const rangeStart = Math.max(2, current - siblings);
  const rangeEnd = Math.min(total - 1, current + siblings);

  // Always include page 1.
  pages.push(1);

  // Ellipsis before range if needed.
  if (rangeStart > 2) {
    pages.push(null);
  }

  // Pages in the middle range.
  for (let p = rangeStart; p <= rangeEnd; p++) {
    pages.push(p);
  }

  // Ellipsis after range if needed.
  if (rangeEnd < total - 1) {
    pages.push(null);
  }

  // Always include the last page.
  pages.push(total);

  return pages;
}

import type { TestCredentialResponse } from "../../types";

export function TestResultBanner({
  result,
  onDismiss,
}: {
  result: TestCredentialResponse | null;
  onDismiss: () => void;
}) {
  if (!result) return null;

  if (result.valid) {
    return (
      <div className="mb-4 flex items-start justify-between rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700">
        <div className="flex items-start gap-2">
          <svg
            className="mt-0.5 h-4 w-4 flex-shrink-0"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
            />
          </svg>
          <div>
            <span className="font-medium">Test passed.</span>
            {result.metadata && Object.keys(result.metadata).length > 0 && (
              <pre className="mt-1 whitespace-pre-wrap text-xs text-green-600">
                {JSON.stringify(result.metadata, null, 2)}
              </pre>
            )}
          </div>
        </div>
        <button
          onClick={onDismiss}
          className="ml-4 flex-shrink-0 rounded p-0.5 text-green-500 hover:bg-green-100 hover:text-green-700"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={1.5}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>
      </div>
    );
  }

  return (
    <div className="mb-4 flex items-start justify-between rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      <div className="flex items-start gap-2">
        <svg
          className="mt-0.5 h-4 w-4 flex-shrink-0"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={2}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
          />
        </svg>
        <div>
          <span className="font-medium">Test failed.</span>
          {result.error && (
            <p className="mt-1 text-xs text-red-600">{result.error}</p>
          )}
        </div>
      </div>
      <button
        onClick={onDismiss}
        className="ml-4 flex-shrink-0 rounded p-0.5 text-red-500 hover:bg-red-100 hover:text-red-700"
      >
        <svg
          className="h-4 w-4"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M6 18L18 6M6 6l12 12"
          />
        </svg>
      </button>
    </div>
  );
}

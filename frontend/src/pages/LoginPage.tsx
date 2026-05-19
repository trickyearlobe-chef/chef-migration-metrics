import { useState, useEffect, type FormEvent } from "react";
import { useAuth } from "../context/AuthContext";
import { fetchAuthInfo } from "../api";

export function LoginPage() {
  const { login, error, loggingIn } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [localEnabled, setLocalEnabled] = useState(true);
  const [samlEnabled, setSamlEnabled] = useState(false);

  useEffect(() => {
    fetchAuthInfo()
      .then((info) => {
        setLocalEnabled(info.local_enabled);
        setSamlEnabled(info.saml_enabled);
      })
      .catch(() => {
        // Default to showing local login on error.
      });
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!username || !password) return;
    await login(username, password);
  }

  function handleSSO() {
    window.location.href = "/api/v1/auth/saml/login";
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-100 px-4">
      <div className="w-full max-w-sm">
        {/* Logo / brand */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-blue-600 text-white">
            <svg
              className="h-7 w-7"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3 1 3"
              />
            </svg>
          </div>
          <h1 className="text-xl font-bold text-gray-800">
            Chef Migration Metrics
          </h1>
          <p className="mt-1 text-sm text-gray-500">
            Sign in to your account
          </p>
        </div>

        {/* Card */}
        <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
          {/* Error banner */}
          {error && (
            <div className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {error}
            </div>
          )}

          {/* SSO button */}
          {samlEnabled && (
            <>
              <button
                type="button"
                onClick={handleSSO}
                className="flex w-full items-center justify-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
              >
                <svg
                  className="h-5 w-5 text-gray-500"
                  fill="none"
                  viewBox="0 0 24 24"
                  strokeWidth={1.5}
                  stroke="currentColor"
                  aria-hidden="true"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z"
                  />
                </svg>
                Sign in with SSO
              </button>
              {localEnabled && (
                <div className="my-4 flex items-center gap-3">
                  <hr className="flex-1 border-gray-200" />
                  <span className="text-xs text-gray-400">or</span>
                  <hr className="flex-1 border-gray-200" />
                </div>
              )}
            </>
          )}

          {/* Local login form */}
          {localEnabled && (
            <form onSubmit={handleSubmit}>
              {/* Username */}
              <label className="mb-4 block">
                <span className="mb-1 block text-sm font-medium text-gray-700">
                  Username
                </span>
                <input
                  type="text"
                  autoComplete="username"
                  autoFocus
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={loggingIn}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 disabled:text-gray-500"
                  placeholder="admin"
                />
              </label>

              {/* Password */}
              <label className="mb-6 block">
                <span className="mb-1 block text-sm font-medium text-gray-700">
                  Password
                </span>
                <input
                  type="password"
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={loggingIn}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 disabled:text-gray-500"
                  placeholder="••••••••"
                />
              </label>

              {/* Submit */}
              <button
                type="submit"
                disabled={loggingIn || !username || !password}
                className="flex w-full items-center justify-center rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {loggingIn ? (
                  <>
                    <svg
                      className="-ml-1 mr-2 h-4 w-4 animate-spin text-white"
                      xmlns="http://www.w3.org/2000/svg"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        className="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        strokeWidth="4"
                      />
                      <path
                        className="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                      />
                    </svg>
                    Signing in…
                  </>
                ) : (
                  "Sign in"
                )}
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}

/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Status colours matching the spec confidence indicators
        'status-compatible': '#16a34a',    // green-600
        'status-cookstyle': '#d97706',    // amber-600
        'status-incompatible': '#dc2626',  // red-600
        'status-untested': '#6b7280',    // gray-500
        'status-stale': '#9333ea',    // purple-600
      },
    },
  },
  plugins: [],
}

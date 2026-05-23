/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ready: { 50: '#ecfdf5', 300: '#6ee7b7', 600: '#059669', 700: '#047857' },
        update: { 50: '#eff6ff', 300: '#93c5fd', 600: '#2563eb', 700: '#1d4ed8' },
        over: { 50: '#fef2f2', 300: '#fca5a5', 600: '#dc2626', 700: '#b91c1c' },
      },
    },
  },
  plugins: [],
};

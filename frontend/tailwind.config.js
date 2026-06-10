/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ops: {
          titlebar: '#161614',
          canvas: '#1A1A19',
          sidebar: '#1F1F1D',
          surface: '#262624',
          elevated: '#2F2F2C',
          input: '#1F1F1D',
          primary: '#F5F4ED',
          secondary: '#A8A6A1',
          tertiary: '#6B6962',
          inverse: '#1A1A19',
          overlay: 'rgba(0,0,0,0.6)',
          border: {
            subtle: '#3A3A36',
            strong: '#4A4A45',
            focus: '#D97757',
          },
          accent: {
            DEFAULT: '#D97757',
            hover: '#C66948',
            soft: 'rgba(217,119,87,0.15)',
          },
          success: {
            DEFAULT: '#10B981',
            soft: 'rgba(16,185,129,0.12)',
          },
          warning: {
            DEFAULT: '#F59E0B',
            soft: 'rgba(245,158,11,0.12)',
          },
          danger: {
            DEFAULT: '#EF4444',
            soft: 'rgba(239,68,68,0.12)',
          },
          info: {
            DEFAULT: '#60A5FA',
            soft: 'rgba(96,165,250,0.12)',
          },
        },
        ready: { 50: '#ecfdf5', 300: '#6ee7b7', 600: '#059669', 700: '#047857' },
        update: { 50: '#eff6ff', 300: '#93c5fd', 600: '#2563eb', 700: '#1d4ed8' },
        over: { 50: '#fef2f2', 300: '#fca5a5', 600: '#dc2626', 700: '#b91c1c' },
      },
      fontFamily: {
        sans: ['Inter', 'PingFang SC', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'SF Mono', 'Cascadia Code', 'monospace'],
      },
      fontSize: {
        '2xs': ['11px', '14px'],
        xs: ['12px', '16px'],
        sm: ['13px', '18px'],
        base: ['15px', '22px'],
        lg: ['18px', '26px'],
        xl: ['22px', '30px'],
      },
    },
  },
  plugins: [],
};

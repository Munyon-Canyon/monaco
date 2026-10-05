/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_MONACO_API_URL: string;
  readonly VITE_PRIVY_APP_ID: string;
  readonly VITE_PRIVY_ENV: "sandbox" | "production";
}

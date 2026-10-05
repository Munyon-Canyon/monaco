import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { PrivyProvider } from "@privy-io/react-auth";
import { App } from "./App";
import { initialState } from "./lib";

// Read the one-time token, then drop it from the address bar so it is never bookmarked or shared.
const initial = initialState(location.search);
history.replaceState(null, "", location.pathname);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <PrivyProvider
      appId={import.meta.env.VITE_PRIVY_APP_ID}
      config={{
        loginMethods: ["sms", "email"],
        // The member wallet already exists and is made by the backend; this page must never create one.
        embeddedWallets: { ethereum: { createOnLogin: "off" }, solana: { createOnLogin: "off" } },
      }}
    >
      <App initial={initial} />
    </PrivyProvider>
  </StrictMode>,
);

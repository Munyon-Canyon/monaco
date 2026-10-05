// The fund page's Content-Security-Policy: Privy's published policy
// (docs.privy.io, security, content security policy), its funding and captcha additions,
// the on-ramp providers Privy hands off to, and the Monaco API the page calls.
const stripe = ["https://js.stripe.com", "https://crypto-js.stripe.com", "https://hooks.stripe.com"];
const providers = [
  ...stripe,
  "https://buy.moonpay.com",
  "https://buy-sandbox.moonpay.com",
  "https://pay.coinbase.com",
  "https://meldcrypto.com",
  "https://*.meld.io",
];
const hcaptcha = ["https://hcaptcha.com", "https://*.hcaptcha.com"];
const walletconnectVerify = ["https://verify.walletconnect.com", "https://verify.walletconnect.org"];

export function fundCSP(apiURL: string): string {
  const directives: Record<string, string[]> = {
    "default-src": ["'self'"],
    "script-src": ["'self'", "https://challenges.cloudflare.com", ...stripe, ...hcaptcha],
    "style-src": ["'self'", "'unsafe-inline'", ...hcaptcha],
    "img-src": ["'self'", "data:", "blob:", "https:"],
    "font-src": ["'self'", "data:"],
    "object-src": ["'none'"],
    "base-uri": ["'self'"],
    "form-action": ["'self'"],
    "child-src": ["https://auth.privy.io", ...walletconnectVerify],
    "frame-src": ["https://auth.privy.io", ...walletconnectVerify, "https://challenges.cloudflare.com", ...hcaptcha, ...providers],
    "connect-src": [
      "'self'",
      new URL(apiURL).origin,
      "https://auth.privy.io",
      "wss://relay.walletconnect.com",
      "wss://relay.walletconnect.org",
      "wss://www.walletlink.org",
      "https://*.rpc.privy.systems",
      "https://explorer-api.walletconnect.com",
      "https://api.relay.link",
      "https://api.testnets.relay.link",
      "https://api.stripe.com",
      "https://api.moonpay.com",
      ...hcaptcha,
    ],
    "worker-src": ["'self'"],
    "manifest-src": ["'self'"],
  };
  return Object.entries(directives)
    .map(([name, sources]) => `${name} ${sources.join(" ")}`)
    .join("; ");
}

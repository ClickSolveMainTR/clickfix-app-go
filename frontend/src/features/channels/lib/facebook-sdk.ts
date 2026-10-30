/**
 * Meta JavaScript SDK loader for WhatsApp Embedded Signup. The SDK is
 * loaded on demand (only when an admin starts the signup), and the
 * session info (WABA id, phone number id) arrives as a postMessage from
 * facebook.com while the login popup is open.
 */

type FBLoginResponse = { authResponse?: { code?: string } | null };

type FacebookSDK = {
  init(options: {
    appId: string;
    autoLogAppEvents: boolean;
    xfbml: boolean;
    version: string;
  }): void;
  login(
    cb: (response: FBLoginResponse) => void,
    options: Record<string, unknown>,
  ): void;
};

declare global {
  interface Window {
    FB?: FacebookSDK;
    fbAsyncInit?: () => void;
  }
}

const SDK_URL = "https://connect.facebook.net/en_US/sdk.js";

let loading: Promise<FacebookSDK> | null = null;

function loadSDK(appId: string, version: string): Promise<FacebookSDK> {
  if (window.FB) return Promise.resolve(window.FB);
  if (loading) return loading;
  loading = new Promise((resolve, reject) => {
    window.fbAsyncInit = () => {
      if (!window.FB) return reject(new Error("Facebook SDK unavailable"));
      window.FB.init({ appId, autoLogAppEvents: true, xfbml: false, version });
      resolve(window.FB);
    };
    const script = document.createElement("script");
    script.src = SDK_URL;
    script.async = true;
    script.crossOrigin = "anonymous";
    script.onerror = () => {
      loading = null;
      reject(new Error("Facebook SDK failed to load"));
    };
    document.body.appendChild(script);
  });
  return loading;
}

export type EmbeddedSignupResult = {
  code: string;
  wabaId: string;
  phoneNumberId: string;
};

/** Runs Embedded Signup and resolves with the code and the chosen assets. */
export async function runEmbeddedSignup(config: {
  app_id: string;
  config_id: string;
  api_version: string;
}): Promise<EmbeddedSignupResult> {
  const fb = await loadSDK(config.app_id, config.api_version || "v21.0");
  let session = { wabaId: "", phoneNumberId: "" };
  const onMessage = (event: MessageEvent) => {
    if (!/^https:\/\/([a-z]+\.)?facebook\.com$/.test(event.origin)) return;
    try {
      const data =
        typeof event.data === "string" ? JSON.parse(event.data) : event.data;
      // FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING: a number that keeps using
      // the WhatsApp Business app (coexistence), as in takwim's signup.
      const finished =
        data?.event === "FINISH" ||
        data?.event === "FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING";
      if (data?.type === "WA_EMBEDDED_SIGNUP" && finished) {
        session = {
          wabaId: String(data.data?.waba_id ?? ""),
          phoneNumberId: String(data.data?.phone_number_id ?? ""),
        };
      }
    } catch {
      // Other facebook.com messages are not JSON.
    }
  };
  window.addEventListener("message", onMessage);
  try {
    const code = await new Promise<string>((resolve, reject) => {
      fb.login(
        (response) => {
          const value = response.authResponse?.code;
          if (value) resolve(value);
          else reject(new Error("cancelled"));
        },
        {
          config_id: config.config_id,
          response_type: "code",
          override_default_response_type: true,
          extras: { setup: {}, sessionInfoVersion: "3" },
        },
      );
    });
    // The FINISH message can land just after the login callback.
    for (let i = 0; i < 10 && !session.wabaId; i++) {
      await new Promise((r) => setTimeout(r, 200));
    }
    return { code, ...session };
  } finally {
    window.removeEventListener("message", onMessage);
  }
}

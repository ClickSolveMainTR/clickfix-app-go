/** CF-126: this optional bundle is loaded only after explicit visitor consent. */
import { record } from "rrweb";

const baseBlock =
  "iframe,canvas,video,audio,img,svg,object,embed,input,textarea,select,[contenteditable],[data-private],[data-clickfix-block],.rr-block";

import { scrub } from "./privacy";

export function start(
  emit: (event: unknown) => void,
  selectors: string[],
): () => void {
  const blockSelector = [baseBlock, ...selectors].join(",");
  // Invalid organization selectors fail closed before rrweb starts.
  document.querySelector(blockSelector);
  const stop = record({
    emit: (event) => emit(scrub(event)),
    maskAllInputs: true,
    maskTextSelector: "*",
    blockSelector,
    recordCanvas: false,
    recordCrossOriginIframes: false,
    collectFonts: false,
    inlineStylesheet: false,
    inlineImages: false,
    checkoutEveryNms: 30000,
  });
  return () => stop?.();
}

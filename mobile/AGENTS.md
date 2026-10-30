# Mobile agent notes

Read `README.md` here first; repo-wide rules are in `../AGENTS.md`.

## Expo changes every SDK: do not trust memory

Before touching an Expo, EAS or React Native API, read the `expo` major in
`package.json` and use the matching docs (`https://docs.expo.dev/versions/v58.0.0/`,
index at `https://docs.expo.dev/llms.txt`).

## Rules

- Install with `npx expo install <pkg>` (SDK-compatible versions), never a bare `npm install <pkg>`.
- Routes live in `src/app/` as one-line files; screens and hooks in `src/features/`.
- Never create or edit `ios/` / `android/` (CNG); configure `app.json` and config plugins.
- After `backend/docs/openapi.yaml` changes: `npm run api:generate` (CI fails on a stale `schema.d.ts`).
- New text: a key in both `src/core/i18n/locales/tr` and `en` (`npm run i18n:check`). Text the panel
  already has (`inbox`, `channels`, `errors`, `notifications`) comes from `frontend/src/locales`.
- Never commit store, team or EAS identifiers from another app; the owner fills in ClickFix's own (README, "Release").
- Before saying done: `npm run typecheck && npm run lint && npm run i18n:check && npm test`.

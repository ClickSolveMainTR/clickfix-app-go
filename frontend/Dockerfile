FROM node:22.22.0-alpine3.23 AS base

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH

RUN corepack enable && corepack prepare pnpm@11.22.0 --activate

WORKDIR /app

FROM base AS deps

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM base AS build

ENV NEXT_TELEMETRY_DISABLED=1

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Sentry source maps (CF-227): only the CI image build passes the token, as a
# BuildKit secret mounted for this one command, so it is never an image
# layer, build arg or env var of the image; org/project are not secret.
# Without the token next.config.ts skips the upload. Browser maps left in
# .next/static are deleted so the runtime image never serves one.
ARG SENTRY_ORG=""
ARG SENTRY_PROJECT=""
RUN --mount=type=secret,id=sentry_auth_token \
    SENTRY_AUTH_TOKEN="$(cat /run/secrets/sentry_auth_token 2>/dev/null || true)" \
    SENTRY_ORG="$SENTRY_ORG" SENTRY_PROJECT="$SENTRY_PROJECT" pnpm build \
    && find .next/static -name '*.map' -type f -delete

FROM node:22.22.0-alpine3.23 AS runtime

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

RUN addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app

WORKDIR /app

COPY --from=build --chown=app:app /app/public ./public
COPY --from=build --chown=app:app /app/.next/standalone ./
COPY --from=build --chown=app:app /app/.next/static ./.next/static

USER app

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=10 \
  CMD ["node", "-e", "fetch('http://127.0.0.1:3000/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"]

CMD ["node", "server.js"]

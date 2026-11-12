FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder

WORKDIR /app/ui

COPY ui/package.json ui/pnpm-lock.yaml ui/pnpm-workspace.yaml ./

RUN npm install -g pnpm && \
    pnpm install --frozen-lockfile

COPY ui/ ./
RUN pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

COPY --from=frontend-builder /app/static ./static
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o mplatform .

FROM gcr.io/distroless/static

WORKDIR /app

COPY --from=backend-builder /app/mplatform .

EXPOSE 8080

CMD ["./mplatform"]

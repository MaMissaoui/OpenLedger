# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26 AS build
WORKDIR /src

# Cache module downloads. (Scaffold uses only the stdlib, so this is a no-op
# until external dependencies are added.)
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/server /server
COPY --from=build /src/db/migrations /migrations
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]

# build
FROM golang:1.22-alpine3.19 AS build

RUN apk update
RUN apk add --no-cache make

COPY . /build
WORKDIR /build

ARG VERSION
ENV VERSION $VERSION

RUN make build

# publish
FROM alpine:3.19

RUN apk update
RUN apk add --no-cache ca-certificates poppler-utils wv unrtf tidyhtml

COPY --from=build /build/cmd/www/ www/ 
COPY --from=build /build/dist/app app

CMD ["/app"]

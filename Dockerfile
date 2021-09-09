FROM coolage/golang-ubuntu:1.0 AS build
WORKDIR /go/src/nfs-check
COPY . .
RUN go build -o /nfs-check .

FROM coolage/duc-ubuntu:2.0
COPY --from=build /nfs-check /nfs-check
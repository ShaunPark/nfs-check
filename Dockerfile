FROM coolage/golang-ubuntu:1.0 AS build
WORKDIR /go/src/nfs-check
COPY . .
RUN go build -o /nfs-check .

FROM ubuntu:18.04
RUN apt update -y && \
    apt install -y duc 

COPY --from=build /nfs-check /nfs-check
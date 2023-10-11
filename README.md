# relayer-svc

The service monitors the locking transactions in Rarimo Core and relay the messages to the target chain if the user has submitted a sufficient fee.

## Build

To build the service image locally, there is a shell script `build.sh` that can be used to build the image:

```bash
sh build.sh
```

It will build the image with the tag `near-saver-svc:latest` which could be used to run the service locally via
Docker or Docker-Compose.

## License
[MIT](./LICENSE)

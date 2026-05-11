Starting from a tail input, there doesn't seem to be a good way to extract and set OTel resource attributes with Fluent Bit 5.0.

### Resource Attributes
Not an exhaustive list, but these may be significant to Azure Monitor

- `deployment.environment.name`
- `deployment.environment`
- `service.namespace`
- `service.name`
- `service.instance.id`
- `telemetry.sdk.name`
- `telemetry.sdk.language`
- `telemetry.sdk.version`
- `service.version`

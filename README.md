# Lumigo Components for OpenTelemetry Collector

This repository contains Lumigo specific components for use with the OpenTelemetry Collector.
Releases are built using the [OpenTelemetry Collector Builder](https://opentelemetry.io/docs/collector/custom-collector/).

What's present in the release build is determined by the contents of [builder-config.yaml](cmd/otelcontribcol/builder-config.yaml).

## Adding new Components

There are two situations where new components may be added:

- Adding an upstream component to the collector
- Adding a new custom Lumigo component

### Adding an upstream component to Collector build

Add the extension, receiver, processor, or exporter to the [builder-config.yaml](cmd/otelcontribcol/builder-config.yaml) file.
Then run the following to create the updated OpenTelemetry Collector build:

```shell
make genotelcontribcol
```

Commit the changes and submit a PR.

### Adding a new custom Lumigo component

Add the desired code into the appropriate directory under either `extension`, `receiver`, `processor`, or `exporter`,
depending on the type of component.

Add the new component to the following files:

- [builder-config.yaml](cmd/otelcontribcol/builder-config.yaml)
  - Including the `replaces` directive to reference the local code
- The root `go.mod` file.
  - Including the `replaces` directive to reference the local code
- `internal/components/components.go`
  - Add to the `import` block
  - Add `NewFactory` usage to the `Components()` function
- In the `modules` section of `versions.yaml`

Run `make` to ensure the code builds correctly.

## How to Release?

### What version to use?

Versioning of this repository will follow the major/minor versioning of the OpenTelemetry Collector.
It *may* also align with the patch versioning, but it will depend on whether there have been changes to the Lumigo components.

For example, if the OpenTelemetry Collector, and Contrib, is at version `v0.97.0`,
then the Lumigo components will be released at version `v0.97.0`.
The patch version of releases will align with the upstream,
unless there have been changes to the Lumigo components requiring a release with the existing major/minor version.

### Release process

Manually create a tag of the `main` branch, and push it to the remote.
The `{version}` should be replaced with the desired version in the format `vX.Y.Z`.

```shell
git tag -a {version} -m "Release {version}"
git push origin refs/tags/{version}
```

Doing the above will trigger a build of the artifacts and create a release.

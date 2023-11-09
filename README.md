# promextractor

A tool to extract certain time series from a [Prometheus](https://prometheus.io/) database, and create new time blocks from them, suitable for putting into another Prometheus instance.
The program does this by iterating over all time series in the input TSDB that match the given filters, and writing all of that data into an [OpenMetrics](https://openmetrics.io/) compatible file.
Blocks are then created using `promtool tsdb create-blocks-from openmetrics`.
Optionally, the program can copy the new blocks into an existing prometheus directory.

Configuration is done by environment variables, mainly:
- `INPUT_TSDB_PATH` - where to extract data from
- `FILTER_LABEL_NAME` - the name of the label to filter on, eg `job` or `endpoint`
- `FILTER_LABEL_EXPRESSION` - the regular expression to filter with
- `EXISTING_TSDB_PATH` - if provided, the new block files (with the extracted data) will be copied to this path

There are additional possible options, see [main.go](./main.go) for more information.

## Running in a cluster

The idea is to have two prometheus instances in the cluster: one with just the data you know you're interested in keeping for a while, and another with much more data (possibly from a [promdump](https://github.com/ihcsim/promdump)) which you want to explore before determining what to save.
The `promextractor` can help extract data from the "raw" prometheus instance, and copy it into the "trimmed" instance.

The example deployment files may help automate this process on a kubernetes cluster:
- `00_normal` represents the existing setup for the two prometheus instances. The other kustomize targets will build off of this, so if your configuration is different, you can make those changes here and hopefully not repeat yourself in the other targets.
- `01_sleep_raw` adjusts the "raw" instance to exist, but not have prometheus running. This is intended to allow `promdump` to be run and push new data in. This step is not needed if you already have your data in the "raw" prometheus.
- `02_sanitize` is just a return to normal. This allows the "raw" prometheus to re-compact its database, and verify that all its data is valid.
- `03_extract` scales down both of the prometheus instances so that the `promextractor` job can read from "raw", do the filtering, and copy the new blocks into the "trimmed" instance. This is the place to customize the setup for the metrics you are interested in. Once the extractor job is complete, you can return the configuration to "normal"

Tip: if the data you want to extract does not all have one label you can match (eg some series are labelled with `job=foo`, and others are labelled with `pod=bar`), you can run the `03_extract` step multiple times with different configurations.

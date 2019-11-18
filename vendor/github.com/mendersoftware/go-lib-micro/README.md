# go-lib-micro
[![Build Status](https://travis-ci.org/mendersoftware/go-lib-micro.svg?branch=master)](https://travis-ci.org/mendersoftware/go-lib-micro)
[![codecov](https://codecov.io/gh/mendersoftware/go-lib-micro/branch/master/graph/badge.svg)](https://codecov.io/gh/mendersoftware/go-lib-micro)
[![Go Report Card](https://goreportcard.com/badge/github.com/mendersoftware/go-lib-micro)](https://goreportcard.com/report/github.com/mendersoftware/go-lib-micro)


Mender: go-lib-micro
==============================================

Mender is an open source over-the-air (OTA) software updater for embedded Linux
devices. Mender comprises a client running at the embedded device, as well as
a server that manages deployments across many devices.

This repository contains the Mender go-lib-micro library, which is part of the
Mender server. The Mender server is designed as a microservices architecture
and comprises several repositories.

The go-lib-micro library is a collection of utilities and middlewares shared among microservices in the Mender ecosystem.


![Mender logo](https://mender.io/user/pages/04.resources/_logos/logoS.png)


## Getting started

To start using Mender, we recommend that you begin with the Getting started
section in [the Mender documentation](https://docs.mender.io/).

## Using the library

The library's code is divided into subpackages, which can be imported the standard Go way:

```
import (
    "github.com/mendersoftware/go-lib-micro/log"
    "github.com/mendersoftware/go-lib-micro/requestid"
)
```

For example usage, please see e.g. the [Mender Deployments Service](https://github.com/mendersoftware/deployments).


## Contributing

We welcome and ask for your contribution. If you would like to contribute to Mender, please read our guide on how to best get started [contributing code or
documentation](https://github.com/mendersoftware/mender/blob/master/CONTRIBUTING.md).

## License

Mender is licensed under the Apache License, Version 2.0. See
[LICENSE](https://github.com/mendersoftware/go-lib-micro/blob/master/LICENSE) for the
full license text.

## Security disclosure

We take security very seriously. If you come across any issue regarding
security, please disclose the information by sending an email to
[security@mender.io](security@mender.io). Please do not create a new public
issue. We thank you in advance for your cooperation.

## Connect with us

* Join our [Google
  group](https://groups.google.com/a/lists.mender.io/forum/#!forum/mender)
* Follow us on [Twitter](https://twitter.com/mender_io?target=_blank). Please
  feel free to tweet us questions.
* Fork us on [Github](https://github.com/mendersoftware)
* Email us at [contact@mender.io](mailto:contact@mender.io)

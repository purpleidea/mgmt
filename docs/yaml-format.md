# mgmt

<!--
Mgmt
Copyright (C) 2013-2016+ James Shubin and the project contributors
Written by James Shubin <james@shubin.ca> and the project contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
-->

## mgmt YAML format
#### Available from:
#### [https://github.com/purpleidea/mgmt/](https://github.com/purpleidea/mgmt/)

#### This documentation is available in: [Markdown](https://github.com/purpleidea/mgmt/blob/master/docs/yaml-format.md) or [PDF](https://pdfdoc-purpleidea.rhcloud.com/pdf/https://github.com/purpleidea/mgmt/blob/master/docs/yaml-format.md) format.

#### Table of Contents

1. [Overiew](#overview)
2. [Resources](#resources)
	* [Version 1](#version-1)
	* [Version 2](#version-2)
	* [Meta Parameters](#meta-parameters)
3. [Edges](#edges)

`mgmt` has an internal YAML format for describing its graph. Until a proper DSL
is defined, this is the native way to describe a `mgmt` graph and this document
contains the necessary information to understand it.

## Overview

The YAML format describes a graph, composed ov vertices, the resources, and
edges between them. The basic structure is as follows

    ---
    graph: NAME
    version:
    resources:
    edges:
    collect:
    comment:
    remote:

- `graph` contains the graph name, a string
- `version` contains the graph version, either 1 (the default) or 2.
- `resources` contains the resources of the graph, the vertices of the graph.
	Its structure depends on the graph version.
- `edges` contains a list of edges between the resources

## Resources

### Version 1

Resources in the V1 format are grouped by the resource kind:

    ---
    resources:
      RESOURCE_KIND:
        - name: RESOURCE_NAME
          meta:
            META_PARAMETERS
          RESOURCE_PARAMETERS
        ...

The resource itself contains:

- `name`: the name of the resource, relative to its kind
- `meta`: the resource meta parameters
- other parameters that are speficied per resource


### Version 2

Resources in the V2 format use the following structure:

    ---
    resources:
      - name: RESOURCE_NAME
        kind: RESOURCE_KIND
        params:
          RESOURCE_PARAMETERS
        before:
          - EDGE_TO_PREVIOUS_RESOURCE
          ...
        after:
          - EDGE_TO_SUCCESSOR_RESOURCE
          ...
        META_PARAMETERS
      ...

The resource itself contains:

- `name`: the name of the resource, relative to its kind
- `kind`: the kind of the resource that define its behaviour
- `params`: the resource parameters defined by the resource kind
- `before` and `after` which are a list od edge definitions, relative to the
	current resource.
- other parameters are interpreted as meta parameters

The edges format is a string that contain in that order:

- The optional character `*` signifies that the edge is notifying
- The other resource kind
- a space character
- The other resource name


### Meta Parameters

Meta parameters are:

- `autoedge`: true if we should we generate auto edges
- `autogroup`: true if we should we auto group
- `noop`: true to render the resource noop
- `retry`: number of times to retry on error. -1 for infinite
- `delay`: number of milliseconds to wait between retries
- `poll`: number of seconds between poll intervals, 0 to watch
- `limit`: number of events per second to allow through
- `burst`: number of events to allow in a burst

## Edges

Edges are defined with the following structure:

    ---
    edges:
      - name: EDGE_NAME
        from:
          kind: RESOURCE_KIND
          name: RESOURCE_NAME
        to:
          kind: RESOURCE_KIND
          name: RESOURCE_NAME
        notify: true|false

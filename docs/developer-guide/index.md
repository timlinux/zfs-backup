<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# Developer Guide

<span class="kz-eyebrow">KARTOZA · ZFS BACKUP</span>

How the tool is put together and how to hack on it.

<div class="grid cards" markdown>

-   :material-graph-outline:{ .lg .middle } __Architecture__

    ---

    Go + Bubble Tea + Lipgloss, with `syncoid` doing the heavy lifting
    underneath. State machine, stages, progress channel.

    [:octicons-arrow-right-24: Architecture](architecture.md)

-   :material-hammer-wrench:{ .lg .middle } __Building__

    ---

    `nix develop` + `go build`, plus the helper `nix run .#foo`
    commands defined by the flake.

    [:octicons-arrow-right-24: Building](building.md)

-   :material-source-pull:{ .lg .middle } __Contributing__

    ---

    Issue template, commit conventions, PR flow, coding standards.

    [:octicons-arrow-right-24: Contributing](contributing.md)

</div>

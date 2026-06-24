<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# License

Kartoza ZFS Backup Tool is open-source software. The full licence text lives
in the repository root:

[:octicons-file-code-24: LICENSE on GitHub](https://github.com/timlinux/zfs-backup/blob/main/LICENSE)

## Third-party components

The TUI builds on the [Charm](https://charm.sh/) ecosystem
([Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lipgloss](https://github.com/charmbracelet/lipgloss),
[Bubbles](https://github.com/charmbracelet/bubbles)).
Data transfer is performed by
[`syncoid`](https://github.com/jimsalterjrs/sanoid) from the Sanoid project.
Snapshots, send/receive and encryption are provided by
[OpenZFS](https://openzfs.org/).

Each third-party component is governed by its own licence; see the project
SBOM published with each release for the full inventory.

<div class="kz-footer-credits" markdown>
Made with 💗 by [Kartoza](https://kartoza.com) &middot;
[Sponsor on GitHub](https://github.com/sponsors/kartoza) &middot;
[Repository](https://github.com/timlinux/zfs-backup)
</div>

Name:           zfs-backup
Version:        1.2.0
Release:        1%{?dist}
Summary:        Beautiful TUI for managing ZFS backups

License:        MIT
URL:            https://github.com/timlinux/zfs-backup
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.22
Requires:       zfs
Requires:       sanoid
Recommends:     udisks2

%description
A beautiful Terminal User Interface (TUI) for managing ZFS backups to external
drives. Features include incremental backups using syncoid, file restoration
from snapshots, pool maintenance with scrub control, and safe device unmounting.

Made with <3 by Kartoza.

%prep
%autosetup

%build
go build -ldflags="-s -w -X main.appVersion=%{version}" -o %{name} .

%install
install -Dm755 %{name} %{buildroot}%{_bindir}/%{name}

%files
%license LICENSE
%doc README.md
%{_bindir}/%{name}

%changelog
* Tue Mar 04 2026 Kartoza <info@kartoza.com> - 1.2.0-1
- Add pool info viewer with scrollable display
- Add pool maintenance with scrub control
- Fix pool import/unlock flow for encrypted pools
- Make result reports scrollable
- Remove emojis for cleaner UI

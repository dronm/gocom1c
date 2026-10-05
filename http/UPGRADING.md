# Upgrading the HTTP Windows service

Upgrade an existing production service by stopping it, replacing its executable,
and starting it again. Keep the executable's registered path and filename the
same. Uninstalling and reinstalling is unnecessary for a normal application
update and would remove the existing service registration.

Keeping the registration preserves the service account, startup type, recovery
settings, and command-line arguments.

The examples assume the installed executable is named `srv1c.exe`. If it was
renamed, substitute its actual filename as well as its installation directory.

1. **Prepare the HTTP executable.**

	Build from the `http` directory, using the same architecture as the current
	application and its installed 1C COM connector:

	```bash
	make prod
	```

	`make prod` builds Windows amd64; `make prod32` builds Windows 386. Both
	produce `srv1c.exe`. The Redis application also produces a file with that
	name, so use the executable built from `http` for the HTTP service. Stage
	the new executable separately from the installed executable.

2. **Check the registered service.**

	Open PowerShell as Administrator:

	```powershell
	$serviceName = "GoCOM1CService";
	sc.exe qc $serviceName;
	```

	If the service has a custom name, use its actual name. Check
	`BINARY_PATH_NAME` for the installed executable and its arguments. The
	application reads `config.json` from the directory containing that executable.

3. **Pause requests and stop the service.**

	Pause clients, scheduled jobs, or other callers and allow active COM commands
	to finish before stopping the service.

	```powershell
	Stop-Service -Name $serviceName -ErrorAction Stop;
	(Get-Service -Name $serviceName).WaitForStatus("Stopped", [TimeSpan]::FromMinutes(2));
	Get-Service -Name $serviceName;
	```

	Proceed after the service is `Stopped` and its process has exited. If stopping
	fails or times out, resolve that before replacing any files. If the executable
	is still locked, wait for the old process to exit.

4. **Back up the installed executable and configuration.**

	For example, after substituting the actual installation directory:

	```powershell
	$appDir = "C:\GoCOM1C";
	$backupDir = Join-Path $appDir ("backup-" + (Get-Date -Format "yyyyMMdd-HHmmss"));
	New-Item -ItemType Directory -Path $backupDir -ErrorAction Stop | Out-Null;
	Copy-Item -LiteralPath (Join-Path $appDir "srv1c.exe") -Destination $backupDir -ErrorAction Stop;
	Copy-Item -LiteralPath (Join-Path $appDir "config.json") -Destination $backupDir -ErrorAction Stop;
	```

	Keep this backup until the upgrade has been verified.

5. **Replace only the application executable.**

	Copy the new HTTP build over the existing `srv1c.exe`, preserving its path
	and filename. For example:

	```powershell
	$newExe = "C:\Temp\srv1c.exe";
	Copy-Item -LiteralPath $newExe -Destination (Join-Path $appDir "srv1c.exe") -Force -ErrorAction Stop;
	```

	Keep the production `config.json`, including its COM connection string,
	authentication settings, and HTTP address. Do not replace it with the sample
	configuration from the source archive.

	For the logging update, `logRotationPeriod` is optional. Omitting it selects
	`"daily"`, which rotates by the server's local calendar date. Fixed durations
	such as `"6h"` or `"24h"` are also supported. File logging requires
	`"logToFile": true`; existing settings remain effective.

	When enabled, HTTP logs are written under `%ProgramData%\GoCom1c\logs`,
	for example `log-2026-10-05.txt`. The service account needs permission to create
	and write files there. Existing logs are preserved and are not automatically
	deleted.

6. **Start and verify the upgraded service.**

	```powershell
	Start-Service -Name $serviceName -ErrorAction Stop;
	(Get-Service -Name $serviceName).WaitForStatus("Running", [TimeSpan]::FromMinutes(2));
	Get-Service -Name $serviceName;
	```

	Check the startup log. If startup fails, also inspect the Windows Application
	Event Log for the service name.

	Test `/health` using the configured HTTP address and port, then test a known
	COM operation. `/health` confirms HTTP availability; it does not execute a
	COM operation or verify the 1C connection. Resume callers after verification.

To roll back, pause callers, stop the service using the same procedure, and
restore the backed-up `srv1c.exe` to its original path. Restore `config.json` only
if configuration was changed during the upgrade. Start the service and repeat
the checks before resuming callers.

If the executable must move to a different path, update the service's registered
binary path with `sc.exe config`, preserving its arguments, account, and other
settings. Keep `config.json` beside the executable at its new location. A normal
upgrade at the same path requires no service registration changes.

The same stop, replace, and start procedure applies to the Redis executable,
using `GoCOM1CRedisService` and the build from `redis`. Pause Redis producers and
drain active work before stopping it. Its updated file logs use
`%ProgramData%\GoCom1c\logs\redis1c-YYYY-MM-DD.log` in daily mode.

Microsoft references:

- [Querying service configuration](https://learn.microsoft.com/en-us/windows/win32/services/configuring-a-service-using-sc)
- [Stop-Service](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/stop-service)
- [Waiting for a service status](https://learn.microsoft.com/en-us/dotnet/api/system.serviceprocess.servicecontroller.waitforstatus)
- [Changing service configuration](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/sc-config)

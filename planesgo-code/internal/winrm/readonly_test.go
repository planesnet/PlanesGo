package winrm

import "testing"

func TestReadOnlyAllowed(t *testing.T) {
	for _, s := range []string{
		`Get-ChildItem 'C:\Program Files'`,
		`Get-Service | Where-Object Status -eq 'Running' | Select-Object Name, DisplayName | Sort-Object Name | Format-Table -AutoSize`,
		"$v = Get-CimInstance Win32_VideoController | Where-Object CurrentHorizontalResolution | Select-Object -First 1; \"$($v.CurrentHorizontalResolution)x$($v.CurrentVerticalResolution)\"",
		`Get-Process | ForEach-Object { $_.Name.ToUpper() } | Measure-Object`,
		`gps | % { $_.Id } | sort -Descending | select -First 3`,
		`ls C:\ -Recurse -Filter *.log -ErrorAction SilentlyContinue | ? { $_.Length -gt 1MB } | ft Name, Length`,
		"if (Test-Path 'C:\\x') {\n  'sí'\n} elseif ($env:COMPUTERNAME -like 'SAGE*') { 'no' } else { exit 3 }",
		"foreach ($f in Get-ChildItem C:\\) {\n  $n = $f.Name.Substring(0, 1)\n  $f.LastWriteTime.AddDays(-1).ToString('yyyy-MM-dd')\n}",
		"for ($i = 0; $i -lt 3; $i++) { $i * 2 }",
		`[math]::Round((Get-Item C:\x.log).Length / 1KB, 2)`,
		`[datetime]::Now.ToString('s'); [int]$n = '5'; $h = @{}; $h['a'] = 1; $a, $b = 1, 2`,
		`Get-ItemProperty HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion | Select-Object ProgramFilesDir`,
		"Get-Content C:\\x.log -Tail 20 2>&1",
		"Get-Item C:\\no > $null",
		"Get-WinEvent -LogName System -MaxEvents 5 | Select-Object TimeCreated, Id, @{n='Mensaje'; e={ $_.Message.Split(\"`n\")[0] }}",
		"# a comment with Remove-Item\nGet-Date <# Stop-Computer #>",
		"'Remove-Item C:\\ -Recurse'; \"Stop-Service x\"",
		"@'\nRemove-Item C:\\\n'@\n@\"\nhoy: $(Get-Date)\n\"@",
		"whoami; hostname.exe; systeminfo | Select-String 'OS'",
		"try { Get-Item C:\\no -ErrorAction Stop } catch [System.Management.Automation.ItemNotFoundException] { 'falta' } finally { 'fin' }",
		"$s = switch ($x) { 'a' { 1 } default { 2 } }",
		"Get-ChildItem `\n  -Path C:\\ `\n  -Force",
		"Get-ChildItem -Path C:\\a,\n  C:\\b",
		"function Get-Size($p) { (Get-Item $p).Length }\nGet-Size C:\\x",
		"$env:TEMP; $global:x = 1; ${a b} ; 1..5 | Measure-Object -Sum",
		"Get-ChildItem C:\\x[1].txt; Get-Process -Name svchost -ErrorAction 0",
		"(Get-Date) - (Get-Date).AddDays(-7)",
	} {
		if why := ReadOnlyCheck(s); why != "" {
			t.Errorf("%q refused: %s", s, why)
		}
	}
}

func TestReadOnlyRefused(t *testing.T) {
	for _, s := range []string{
		`Remove-Item C:\x.log`,
		`Get-ChildItem C:\tmp | Remove-Item`,
		"Get-Service x |\n  Stop-Service",
		`Stop-Computer -Force`,
		`Set-ItemProperty HKLM:\x -Name a -Value 1`,
		`New-Item C:\x -ItemType File`,
		`Invoke-Expression 'Remove-Item C:\x'`,
		`iex 'Remove-Item C:\x'`,
		`& 'Remove-Item' C:\x`,
		`& { Get-Date }`,
		`. C:\scripts\x.ps1`,
		`.\x.ps1`,
		`C:\Windows\System32\cmd.exe /c del C:\x`,
		`cmd /c del C:\x`,
		"Re`move-Item C:\\x",
		`"$(Remove-Item C:\x)"`,
		"@\"\n$(Stop-Service x)\n\"@",
		`Get-ChildItem a$(Remove-Item C:\x)`,
		`(Get-Item C:\x.log).Delete()`,
		`Get-Process notepad | ForEach-Object { $_.Kill() }`,
		`Get-Process notepad | % Kill`,
		`Get-Process notepad | ForEach-Object -MemberName Kill`,
		`$p = Get-Process notepad; $p.Kill()`,
		`[IO.File]::WriteAllText('C:\x', 'y')`,
		`[System.IO.File]::Delete('C:\x')`,
		`[scriptblock]::Create('Remove-Item C:\x').Invoke()`,
		`$f = Get-Item C:\x; $f.Attributes = 'Hidden'`,
		`(Get-Item C:\x).IsReadOnly = $true`,
		`${C:\x.txt} = 'hola'`,
		`$function:Get-X = { 1 }`,
		`Get-Date > C:\x.txt`,
		`Get-Date >> C:\x.txt`,
		`Get-Date | Out-File C:\x.txt`,
		`Get-Date | Tee-Object C:\x.txt`,
		`Get-Item C:\x | Export-Csv C:\y.csv`,
		`$m = 'Delete'; (Get-Item C:\x).$m()`,
		`Test-ComputerSecureChannel -Repair`,
		`Start-Process notepad`,
		`$x = New-Object -ComObject WScript.Shell`,
		"class X { [void] F() { } }",
		"using module Peligroso",
		"Get-Date; Rem\u2013ove",
		"\u2018Remove-Item\u2019",
		"function F { [CmdletBinding()] param() Remove-Item C:\\x }",
		"[ValidateScript({ Remove-Item C:\\x })]$x = 1",
		"Get-Date | Where-Object { $_ } | ForEach-Object { Remove-Item C:\\x }",
		"if (1) { Restart-Service w32time }",
		"$x = @{ a = (Remove-Item C:\\x) }",
		"Get-Item C:\\x | Invoke-WmiMethod -Name Delete",
		"(Get-WmiObject Win32_Process -Filter \"Name='notepad.exe'\").Terminate()",
		"Get-ChildItem C:\\ | Where-Object { $_.Delete() }",
		"$x.Count++",
		"Get-Date; (",
		"Get-Date }",
		"'sin cerrar",
	} {
		if ReadOnlyCheck(s) == "" {
			t.Errorf("%q must be refused when read only", s)
		}
	}
}

func TestReadOnlyMessages(t *testing.T) {
	for s, want := range map[string]string{
		`Remove-Item C:\x`:         "«Remove-Item» no se admite en solo lectura",
		`(Get-Item C:\x).Delete()`: "el método .Delete() no se admite",
		`Get-Date > C:\x`:          "redirigir la salida",
	} {
		if got := ReadOnlyCheck(s); len(got) < len(want) || got[:len(want)] != want {
			t.Errorf("%q: %q, want %q…", s, got, want)
		}
	}
}

// The scanner never hangs or panics, whatever it reads.
func FuzzReadOnlyCheck(f *testing.F) {
	for _, s := range []string{"Get-Date", "\"$(\"", "@'\n", "[a[b]", "$", "`", "${", "a.", "-", "--%", "@\"\n$(x)\n\"@"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { ReadOnlyCheck(s) })
}

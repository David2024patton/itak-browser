Set WshShell = CreateObject("WScript.Shell")
WshShell.Run chr(34) & "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" & Chr(34) & " daemon start", 0
Set WshShell = Nothing

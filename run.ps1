<#
.SYNOPSIS
    编译并运行 gonav 内部网址导航。

.NOTES
    本文件必须保存为「UTF-8 with BOM」：Windows PowerShell 5.1 会把无 BOM 的
    UTF-8 脚本按本地代码页(GBK)解码，中文串错位后可能吞掉引号导致整段解析失败。

.EXAMPLE
    .\run.ps1                       # 只监听本机 :8080
    .\run.ps1 -Lan                  # 监听 0.0.0.0，局域网/手机可访问（并打印可用地址）
    .\run.ps1 -Lan -Port 8123
    .\run.ps1 -Addr 127.0.0.1:9000
    .\run.ps1 -DevMode -Watch 0
#>
[CmdletBinding()]
param(
    [string]$Addr,
    [int]$Port = 8080,
    [string]$Config = "config/links.json",
    [string]$Stats = "data/stats.json",
    [string]$Watch = "3s",
    # 监听所有网卡，供局域网内其它设备访问
    [switch]$Lan,
    # 注意：不能叫 -Debug，那是 CmdletBinding 提供的公共参数。
    [Alias("Dev")]
    [switch]$DevMode,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

if (-not $Addr) {
    $Addr = if ($Lan) { "0.0.0.0:$Port" } else { ":$Port" }
}

if (-not $SkipBuild) {
    Write-Host "[gonav] go build ..." -ForegroundColor Cyan
    go build -trimpath -o gonav.exe .
    if ($LASTEXITCODE -ne 0) { throw "构建失败" }
}

$arguments = @(
    "-addr", $Addr,
    "-config", $Config,
    "-stats", $Stats,
    "-watch", $Watch
)
if ($DevMode) {
    $arguments += @("-debug", "-web-root", "internal/web")
}

if ($Lan) {
    $bindPort = ($Addr -split ":")[-1]
    Write-Host "[gonav] 局域网地址：" -ForegroundColor Green
    Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object {
            $_.IPAddress -notlike "127.*" -and
            $_.IPAddress -notlike "169.254.*" -and
            $_.PrefixOrigin -ne "WellKnown" -and
            $_.InterfaceAlias -notmatch "VMware|VMnet|Hyper-V|vEthernet|Loopback|Bluetooth|虚拟|蓝牙"
        } |
        ForEach-Object { Write-Host ("  http://{0}:{1}/   ({2})" -f $_.IPAddress, $bindPort, $_.InterfaceAlias) }

    # 只对名字里带 gonav 或端口号的规则做端口过滤，避免遍历全部规则（CIM 调用很慢）
    $rule = Get-NetFirewallRule -ErrorAction SilentlyContinue |
        Where-Object {
            $_.Enabled -eq "True" -and $_.Direction -eq "Inbound" -and $_.Action -eq "Allow" -and
            ($_.DisplayName -like "*gonav*" -or $_.DisplayName -like "*$bindPort*")
        } |
        Where-Object { ($_ | Get-NetFirewallPortFilter).LocalPort -contains $bindPort }
    if (-not $rule) {
        $cmd = 'New-NetFirewallRule -DisplayName "gonav (TCP {0})" -Direction Inbound -Protocol TCP -LocalPort {0} -Action Allow -Profile Any' -f $bindPort
        Write-Warning "没有找到放行 TCP $bindPort 的入站规则，其它设备可能连不上。以管理员身份执行："
        Write-Warning "  $cmd"
    }
} else {
    Write-Host "[gonav] 启动 http://127.0.0.1:$Port/  (Ctrl+C 退出)" -ForegroundColor Green
}

& "$PSScriptRoot\gonav.exe" @arguments

/**
 * desktop automation —— 截图 + 键鼠 + 窗口控制（配套使用）
 * Windows 优先，基于 PowerShell + .NET / user32.dll，无原生编译依赖
 */
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { OpenAIDePlugin } from '@openaide/plugins';

const execFileAsync = promisify(execFile);

async function ps(script: string): Promise<string> {
  const { stdout } = await execFileAsync('powershell.exe', ['-NoProfile', '-Command', script], {
    encoding: 'utf8',
    maxBuffer: 10 * 1024 * 1024,
    windowsHide: true,
  } as any);
  return stdout as string;
}

export const desktopPlugin: OpenAIDePlugin = {
  name: 'desktop',
  version: '0.3.0',
  description: '桌面自动化：截图 + 键鼠 + 窗口控制（配套使用）',
  category: 'capability',
  tools: [
    {
      name: 'desktop_screenshot',
      description: '截取屏幕，返回图片路径（vision 模型可直接查看）',
      parameters: {
        type: 'object',
        properties: {
          region: {
            type: 'object',
            description: '可选区域 {x,y,width,height}，默认全屏',
            properties: {
              x: { type: 'number' },
              y: { type: 'number' },
              width: { type: 'number' },
              height: { type: 'number' },
            },
          },
        },
      },
      handler: async (args) => {
        if (process.platform !== 'win32') {
          return { content: '', error: 'desktop_screenshot only supported on Windows currently', errorCode: 'NOT_FOUND' };
        }
        const region = args.region as any;
        const outPath = join(mkdtempSync(join(tmpdir(), 'openaide-screenshot-')), 'screenshot.png');
        // Use .NET System.Drawing
        const script = region
          ? `
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms
$bounds = [System.Drawing.Rectangle]::FromLTRB(${region.x}, ${region.y}, ${region.x + region.width}, ${region.y + region.height})
$bmp = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
$bmp.Save('${outPath.replace(/\\/g, '\\\\')}')
$g.Dispose(); $bmp.Dispose()
Write-Output "${outPath.replace(/\\/g, '\\\\')}"
`
          : `
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms
$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bmp = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
$bmp.Save('${outPath.replace(/\\/g, '\\\\')}')
$g.Dispose(); $bmp.Dispose()
Write-Output "${outPath.replace(/\\/g, '\\\\')}"
`;
        try {
          const stdout = await ps(script);
          const saved = stdout.trim() || outPath;
          return { content: JSON.stringify({ path: saved, note: 'screenshot saved, vision model can view via file path' }, null, 2) };
        } catch (e: any) {
          return { content: '', error: String(e.message), errorCode: 'EXEC_FAILED' };
        }
      },
    },
    {
      name: 'desktop_click',
      description: '鼠标点击指定坐标',
      parameters: {
        type: 'object',
        properties: {
          x: { type: 'number', description: '屏幕 X 坐标' },
          y: { type: 'number', description: '屏幕 Y 坐标' },
          button: { type: 'string', enum: ['left', 'right', 'middle'], description: '按键，默认 left' },
          clicks: { type: 'number', description: '点击次数，默认 1（2 为双击）' },
        },
        required: ['x', 'y'],
      },
      handler: async (args) => {
        if (process.platform !== 'win32') return { content: '', error: 'only Windows supported', errorCode: 'NOT_FOUND' };
        const x = Number(args.x), y = Number(args.y);
        const clicks = Number(args.clicks ?? 1);
        const button = String(args.button ?? 'left');
        const btnMap: Record<string, number> = { left: 0x02, right: 0x08, middle: 0x20 };
        // Use user32 mouse_event via Add-Type
        const script = `
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint cButtons, UIntPtr dwExtraInfo);
}
"@
[Mouse]::SetCursorPos(${x}, ${y})
Start-Sleep -Milliseconds 50
for ($i=0; $i -lt ${clicks}; $i++) {
  if ("${button}" -eq "left") { [Mouse]::mouse_event(0x02,0,0,0,[UIntPtr]::Zero); Start-Sleep -Milliseconds 30; [Mouse]::mouse_event(0x04,0,0,0,[UIntPtr]::Zero) }
  elseif ("${button}" -eq "right") { [Mouse]::mouse_event(0x08,0,0,0,[UIntPtr]::Zero); Start-Sleep -Milliseconds 30; [Mouse]::mouse_event(0x10,0,0,0,[UIntPtr]::Zero) }
  else { [Mouse]::mouse_event(0x20,0,0,0,[UIntPtr]::Zero); Start-Sleep -Milliseconds 30; [Mouse]::mouse_event(0x40,0,0,0,[UIntPtr]::Zero) }
  Start-Sleep -Milliseconds 80
}
Write-Output "clicked ${x},${y} x${clicks} ${button}"
`;
        try {
          const out = await ps(script);
          return { content: out.trim() };
        } catch (e: any) {
          return { content: '', error: String(e.message), errorCode: 'EXEC_FAILED' };
        }
      },
    },
    {
      name: 'desktop_type',
      description: '键盘输入文本（向当前焦点窗口）',
      parameters: {
        type: 'object',
        properties: {
          text: { type: 'string', description: '要输入的文本' },
        },
        required: ['text'],
      },
      handler: async (args) => {
        if (process.platform !== 'win32') return { content: '', error: 'only Windows supported', errorCode: 'NOT_FOUND' };
        const text = String(args.text ?? '');
        // Escape for PowerShell single quotes by doubling
        const escaped = text.replace(/'/g, "''");
        const script = `
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('${escaped}')
Write-Output "typed ${text.length} chars"
`;
        try {
          const out = await ps(script);
          return { content: out.trim() };
        } catch (e: any) {
          return { content: '', error: String(e.message), errorCode: 'EXEC_FAILED' };
        }
      },
    },
    {
      name: 'desktop_key',
      description: '按下单个按键或组合键（如 Enter, Tab, Ctrl+C）',
      parameters: {
        type: 'object',
        properties: {
          key: { type: 'string', description: '按键名，如 Enter, Esc, Tab, F5, a' },
          modifiers: { type: 'array', items: { type: 'string' }, description: '修饰键，如 ["ctrl","shift"]' },
        },
        required: ['key'],
      },
      handler: async (args) => {
        if (process.platform !== 'win32') return { content: '', error: 'only Windows supported', errorCode: 'NOT_FOUND' };
        const key = String(args.key ?? '');
        const mods = (args.modifiers as string[] | undefined) ?? [];
        // Map to SendKeys syntax: ^=ctrl, +=shift, %=alt
        const modMap: Record<string, string> = { ctrl: '^', shift: '+', alt: '%' };
        const prefix = mods.map(m => modMap[m.toLowerCase()] ?? '').join('');
        const keyMap: Record<string, string> = {
          Enter: '{ENTER}', Esc: '{ESC}', Escape: '{ESC}', Tab: '{TAB}', Backspace: '{BACKSPACE}',
          Delete: '{DEL}', Home: '{HOME}', End: '{END}', PageUp: '{PGUP}', PageDown: '{PGDN}',
          Up: '{UP}', Down: '{DOWN}', Left: '{LEFT}', Right: '{RIGHT}', F1: '{F1}', F2: '{F2}', F3: '{F3}', F4: '{F4}',
          F5: '{F5}', F6: '{F6}', F7: '{F7}', F8: '{F8}', F9: '{F9}', F10: '{F10}', F11: '{F11}', F12: '{F12}',
        };
        const sendKey = keyMap[key] ?? key;
        const send = prefix + sendKey;
        const escaped = send.replace(/'/g, "''");
        const script = `
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('${escaped}')
Write-Output "key ${send}"
`;
        try {
          const out = await ps(script);
          return { content: out.trim() };
        } catch (e: any) {
          return { content: '', error: String(e.message), errorCode: 'EXEC_FAILED' };
        }
      },
    },
    {
      name: 'desktop_windows',
      description: '列出当前所有窗口（标题、句柄、进程）',
      parameters: { type: 'object', properties: {} },
      handler: async () => {
        if (process.platform !== 'win32') return { content: '', error: 'only Windows supported', errorCode: 'NOT_FOUND' };
        const script = `
Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public class Win {
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);
  [DllImport("user32.dll")] public static extern int GetWindowText(IntPtr hWnd, StringBuilder lpString, int nMaxCount);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);
  public delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);
}
"@
$list = @()
[Win]::EnumWindows({ param($h,$l)
  if (-not [Win]::IsWindowVisible($h)) { return $true }
  $sb = New-Object System.Text.StringBuilder 512
  [Win]::GetWindowText($h, $sb, 512) | Out-Null
  $title = $sb.ToString()
  if ($title) {
    $pid = 0; [Win]::GetWindowThreadProcessId($h, [ref]$pid) | Out-Null
    $pname = (Get-Process -Id $pid -ErrorAction SilentlyContinue).ProcessName
    $script:list += [PSCustomObject]@{ handle=$h.ToInt64(); title=$title; pid=$pid; process=$pname }
  }
  return $true
}, [IntPtr]::Zero) | Out-Null
$list | Select-Object -First 30 | ConvertTo-Json -Depth 2
`;
        try {
          const out = await ps(script);
          const parsed = JSON.parse(out || '[]');
          return { content: JSON.stringify({ windows: parsed }, null, 2) };
        } catch (e: any) {
          return { content: '', error: String(e.message), errorCode: 'EXEC_FAILED' };
        }
      },
    },
  ],
};

#!/bin/bash
# 最小验证 DMG —— 只用 macOS 自带工具(hdiutil + Finder 布局),零第三方依赖。
#
# 目的:不编全量 app,就让你在真实 Finder 里看到挂载后的最终态 ——
#   红黄绿灯 / 标题栏(卷宗名)/ 背景图 / 图标位置 / Finder 边框,全部 1:1。
#   唯一是假的:里面的 Memoh.app 是占位壳,双击只弹个说明,不是真 Memoh。
#
# 它发现的图标坐标 / 窗口尺寸,会直接喂给 Phase 2 的 electron-builder.yml,
# 所以这脚本是设计迭代的快速循环,不是一次性的。
#
# 用法:  bash make-preview-dmg.sh
# 产物:  dmg/out/Memoh-Install-preview.dmg

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
BUILD="$(cd "$HERE/../build" && pwd)"
OUT="$HERE/out"
VOL="Install Memoh"           # = Finder 窗口标题栏文字
APP_NAME="Memoh"              # Finder 隐藏 .app 后缀,显示为 Memoh
WIN_W=660                     # 窗口内容区逻辑宽(= 背景图逻辑宽)
WIN_H=440                     # 窗口内容区逻辑高
TITLEBAR=28                   # 标题栏高度(隐藏工具栏后约 28pt)
ICON_SIZE=128
ICON_X=330                    # 图标中心 x(窗口居中)
ICON_Y=254                    # 图标中心 y(贴近文字,与原型 --icon-cy 一致)

mkdir -p "$OUT"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
STAGE="$WORK/stage"
mkdir -p "$STAGE"

echo "==> 1/5 占位 Memoh.app(带真图标)"
APP="$STAGE/$APP_NAME.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BUILD/icon.icns" "$APP/Contents/Resources/icon.icns"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$APP_NAME</string>
  <key>CFBundleDisplayName</key><string>$APP_NAME</string>
  <key>CFBundleIdentifier</key><string>ai.memoh.desktop.preview</string>
  <key>CFBundleVersion</key><string>0.0.0</string>
  <key>CFBundleShortVersionString</key><string>0.0.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>$APP_NAME</string>
  <key>CFBundleIconFile</key><string>icon.icns</string>
</dict>
</plist>
PLIST
cat > "$APP/Contents/MacOS/$APP_NAME" <<'EXE'
#!/bin/bash
osascript -e 'display dialog "这是 DMG 布局预览的占位壳,不是真的 Memoh。" buttons {"OK"} default button 1 with icon note with title "Memoh (preview)"'
EXE
chmod +x "$APP/Contents/MacOS/$APP_NAME"

echo "==> 2/5 背景图(WebKit 渲染 -> 1320x880 @2x)"
mkdir -p "$STAGE/.background"
swift "$HERE/render.swift" "$HERE/prototype.html" "$STAGE/.background/bg.png" "$WIN_W" "$WIN_H" 1
# 盖 144 dpi,让 Finder 按 660x440 逻辑点显示这张 1320x880 物理像素图
sips -s dpiHeight 144 -s dpiWidth 144 "$STAGE/.background/bg.png" >/dev/null

echo "==> 3/5 创建可写 DMG"
hdiutil create -srcfolder "$STAGE" -volname "$VOL" -fs HFS+ \
  -format UDRW -ov "$WORK/rw.dmg" >/dev/null
DEV="$(hdiutil attach "$WORK/rw.dmg" -readwrite -noverify -noautoopen | egrep '^/dev/' | head -1 | awk '{print $1}')"
MOUNT="/Volumes/$VOL"

echo "==> 4/5 Finder 布局(背景图 + 图标坐标)"
# 标题栏占高,bounds 高度 = 内容高 + 标题栏。bounds 原点先放在屏幕 {180,140}。
X1=180; Y1=140
X2=$(( X1 + WIN_W ))
Y2=$(( Y1 + WIN_H + TITLEBAR ))
set +e
osascript <<APPLESCRIPT
tell application "Finder"
  tell disk "$VOL"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set the bounds of container window to {$X1, $Y1, $X2, $Y2}
    set vopts to the icon view options of container window
    set arrangement of vopts to not arranged
    set icon size of vopts to $ICON_SIZE
    set text size of vopts to 13
    set background picture of vopts to file ".background:bg.png"
    set position of item "$APP_NAME.app" of container window to {$ICON_X, $ICON_Y}
    update without registering applications
    delay 1
    close
  end tell
end tell
APPLESCRIPT
RC=$?
set -e
if [ "$RC" -ne 0 ]; then
  echo "!! Finder 布局失败(很可能是 macOS 自动化授权被挡 / 错误 -1743)。"
  echo "!! DMG 仍会生成,但没有自定义背景。请在你自己的终端跑这个脚本并同意授权框。"
fi

echo "==> 5/5 压缩成只读 DMG"
sync
hdiutil detach "$DEV" >/dev/null || hdiutil detach "$DEV" -force >/dev/null
rm -f "$OUT/Memoh-Install-preview.dmg"
hdiutil convert "$WORK/rw.dmg" -format UDZO -imagekey zlib-level=9 \
  -o "$OUT/Memoh-Install-preview.dmg" >/dev/null

echo
echo "完成: $OUT/Memoh-Install-preview.dmg"
echo "双击它挂载,看真实 Finder 效果。"

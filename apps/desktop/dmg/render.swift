// HTML → PNG 截图器(macOS 系统自带 WebKit,零第三方依赖)。
//
// 用途:把 dmg/prototype.html 渲染成 DMG 背景 PNG。比裸 Chrome 稳,
// 且和 Electron app 同属 WebKit 系,保真度高。
//
// 用法: swift render.swift <input.html> <output.png> <logicalW> <logicalH> <scale>
//   例:  swift render.swift prototype.html /tmp/bg@2x.png 660 440 2
//
// 实现要点:
//  - 离屏 WKWebView,frame = 逻辑尺寸 × scale。
//  - pageZoom = scale,让文字在更高分辨率下重新栅格化(清晰,非放大模糊)。
//  - didFinish 后注入 export 态(隐藏占位图标 + 纯白底),再 takeSnapshot。
//  - 自带 15s 超时兜底,绝不挂死。

import Cocoa
import WebKit

let argv = CommandLine.arguments
guard argv.count >= 5 else {
    FileHandle.standardError.write("usage: render.swift <in.html> <out.png> <W> <H> [scale]\n".data(using: .utf8)!)
    exit(2)
}
let inputPath = argv[1]
let outputPath = argv[2]
let logicalW = Double(argv[3]) ?? 660
let logicalH = Double(argv[4]) ?? 440
let scale = Double(argv.count > 5 ? argv[5] : "2") ?? 2

let pixelW = logicalW * scale
let pixelH = logicalH * scale

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

final class Renderer: NSObject, WKNavigationDelegate {
    let webView: WKWebView
    init(frame: NSRect, zoom: CGFloat) {
        let cfg = WKWebViewConfiguration()
        webView = WKWebView(frame: frame, configuration: cfg)
        webView.pageZoom = zoom
        super.init()
        webView.navigationDelegate = self
    }

    func webView(_ wv: WKWebView, didFinish nav: WKNavigation!) {
        // 进入导出态:隐藏占位图标,纯白底,确保截图无残留。
        let js = "document.body.classList.add('export');document.documentElement.style.background='#fff';document.body.style.background='#fff';"
        wv.evaluateJavaScript(js) { _, _ in
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) {
                let snap = WKSnapshotConfiguration()
                snap.rect = wv.bounds
                wv.takeSnapshot(with: snap) { image, error in
                    guard let image = image else {
                        FileHandle.standardError.write("snapshot failed: \(String(describing: error))\n".data(using: .utf8)!)
                        exit(4)
                    }
                    guard let tiff = image.tiffRepresentation,
                          let rep = NSBitmapImageRep(data: tiff),
                          let png = rep.representation(using: .png, properties: [:]) else {
                        FileHandle.standardError.write("encode failed\n".data(using: .utf8)!)
                        exit(5)
                    }
                    do {
                        try png.write(to: URL(fileURLWithPath: outputPath))
                        FileHandle.standardError.write("wrote \(rep.pixelsWide)x\(rep.pixelsHigh) -> \(outputPath)\n".data(using: .utf8)!)
                        exit(0)
                    } catch {
                        FileHandle.standardError.write("write failed: \(error)\n".data(using: .utf8)!)
                        exit(6)
                    }
                }
            }
        }
    }

    func webView(_ wv: WKWebView, didFail nav: WKNavigation!, withError error: Error) {
        FileHandle.standardError.write("nav failed: \(error)\n".data(using: .utf8)!)
        exit(7)
    }
}

let frame = NSRect(x: 0, y: 0, width: pixelW, height: pixelH)
let renderer = Renderer(frame: frame, zoom: CGFloat(scale))

let inputURL = URL(fileURLWithPath: inputPath)
renderer.webView.loadFileURL(inputURL, allowingReadAccessTo: inputURL.deletingLastPathComponent())

// 超时兜底:任何卡顿 15s 后强退,绝不无限挂。
DispatchQueue.main.asyncAfter(deadline: .now() + 15) {
    FileHandle.standardError.write("timeout\n".data(using: .utf8)!)
    exit(8)
}

app.run()

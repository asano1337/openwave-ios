import AVFoundation
import SwiftUI
import UIKit
import WebKit

struct OpenWaveView: View {
    private let backendURL = URL(string: "https://openwave-ios.onrender.com")!
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        OpenWaveWebView(url: backendURL, onChangeServer: {})
            .background(Color.black)
            .preferredColorScheme(.dark)
            .onAppear {
                configureAudioSession()
            }
            .onChange(of: scenePhase) { phase in
                if phase == .active || phase == .background {
                    configureAudioSession()
                }
            }
    }

    private func configureAudioSession() {
        let session = AVAudioSession.sharedInstance()
        try? session.setCategory(
            .playback,
            mode: .default,
            options: [.allowAirPlay, .allowBluetoothA2DP]
        )
        try? session.setActive(true, options: [])
        UIApplication.shared.beginReceivingRemoteControlEvents()
    }
}

struct OpenWaveWebView: UIViewRepresentable {
    let url: URL
    let onChangeServer: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeUIView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.allowsInlineMediaPlayback = true
        configuration.allowsPictureInPictureMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = []
        configuration.websiteDataStore = .default()

        let view = WKWebView(frame: .zero, configuration: configuration)
        view.backgroundColor = .black
        view.isOpaque = true
        view.scrollView.contentInsetAdjustmentBehavior = .automatic
        view.navigationDelegate = context.coordinator
        view.allowsLinkPreview = false
        view.allowsBackForwardNavigationGestures = true
        view.load(URLRequest(url: url, cachePolicy: .useProtocolCachePolicy))
        return view
    }

    func updateUIView(_ view: WKWebView, context: Context) {}

    final class Coordinator: NSObject, WKNavigationDelegate {
        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            print("OpenWave navigation error:", error.localizedDescription)
        }

        func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
                     decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            // Keep normal navigation inside the app; external links can still open
            // when the web UI explicitly requests a new window.
            decisionHandler(.allow)
        }
    }
}
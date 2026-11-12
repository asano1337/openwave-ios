import AVFoundation
import SwiftUI
import WebKit

struct OpenWaveView: View {
    private let backendURL = URL(string: "https://openwave-ios.onrender.com")!

    var body: some View {
        OpenWaveWebView(url: backendURL, onChangeServer: {})
            .ignoresSafeArea()
            .background(Color.black)
            .preferredColorScheme(.dark)
            .onAppear {
                try? AVAudioSession.sharedInstance().setCategory(
                    .playback,
                    mode: .default,
                    options: [.allowAirPlay, .allowBluetoothA2DP]
                )
                try? AVAudioSession.sharedInstance().setActive(true)
            }
    }
}

struct OpenWaveWebView: UIViewRepresentable {
    let url: URL
    let onChangeServer: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeUIView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.allowsInlineMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = []
        configuration.websiteDataStore = .default()

        let view = WKWebView(frame: .zero, configuration: configuration)
        view.backgroundColor = .black
        view.isOpaque = true
        view.scrollView.contentInsetAdjustmentBehavior = .never
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
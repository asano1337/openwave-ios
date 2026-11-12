import AVFoundation
import SwiftUI
import WebKit

struct OpenWaveView: View {
    @AppStorage("openwaveBackendURL") private var savedURL = ""
    @State private var draftURL = ""

    var body: some View {
        Group {
            if let url = normalizedURL(savedURL) {
                OpenWaveWebView(url: url, onChangeServer: { savedURL = "" })
                    .ignoresSafeArea()
            } else {
                serverSetup
            }
        }
        .background(Color.black)
        .preferredColorScheme(.dark)
        .onAppear {
            draftURL = savedURL
            try? AVAudioSession.sharedInstance().setCategory(
                .playback,
                mode: .default,
                options: [.allowAirPlay, .allowBluetoothA2DP]
            )
            try? AVAudioSession.sharedInstance().setActive(true)
        }
    }

    private var serverSetup: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text("OPENWAVE")
                .font(.caption.weight(.bold))
                .tracking(3)
                .foregroundStyle(.secondary)
            Text("Подключи свою волну")
                .font(.system(size: 38, weight: .bold, design: .rounded))
            Text("Укажи HTTPS-адрес OpenWave backend. Для проверки в одной Wi‑Fi сети можно использовать адрес компьютера, например http://192.168.1.25:порт.")
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            TextField("https://openwave.example или http://192.168.1.25:порт", text: $draftURL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .keyboardType(.URL)
                .padding(14)
                .background(Color.white.opacity(0.08))
                .clipShape(RoundedRectangle(cornerRadius: 10))
            Button("Открыть OpenWave") {
                if normalizedURL(draftURL) != nil {
                    savedURL = draftURL.trimmingCharacters(in: .whitespacesAndNewlines)
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(.white)
            .foregroundStyle(.black)
            Text("Приложение не запускает Windows EXE внутри iPhone. Backend должен быть доступен по введённому адресу.")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .padding(24)
        .frame(maxWidth: 620)
    }

    private func normalizedURL(_ raw: String) -> URL? {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = URL(string: value),
              let scheme = url.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              url.host != nil else { return nil }
        return url
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
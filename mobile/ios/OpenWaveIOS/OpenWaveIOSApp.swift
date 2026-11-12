import SwiftUI

@main
struct OpenWaveIOSApp: App {
    var body: some Scene {
        WindowGroup {
            OpenWaveView()
                .preferredColorScheme(.dark)
        }
    }
}
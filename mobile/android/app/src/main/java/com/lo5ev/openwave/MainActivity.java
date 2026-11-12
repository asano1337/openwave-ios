package com.lo5ev.openwave;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.graphics.Color;
import android.os.Build;
import java.io.*;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private WebView webView;
    private Process server;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(Color.rgb(16,17,17));
        getWindow().setNavigationBarColor(Color.rgb(16,17,17));
        webView = new WebView(this);
        webView.setBackgroundColor(Color.rgb(16,17,17));
        WebSettings s = webView.getSettings();
        s.setJavaScriptEnabled(true); s.setDomStorageEnabled(true); s.setMediaPlaybackRequiresUserGesture(false);
        s.setAllowFileAccess(false); s.setAllowContentAccess(false);
        webView.setWebViewClient(new WebViewClient());
        setContentView(webView);
        startLocalServer();
    }

    private void startLocalServer() {
        Executors.newSingleThreadExecutor().execute(() -> {
            try {
                String abi = Build.SUPPORTED_ABIS.length == 0 ? "" : Build.SUPPORTED_ABIS[0];
                String asset = abi.contains("arm64") ? "openwave-server-arm64"
                        : abi.contains("armeabi") ? "openwave-server-arm"
                        : abi.contains("x86_64") ? "openwave-server-amd64"
                        : "openwave-server-386";
                File bin = new File(getFilesDir(), asset);
                if (!bin.exists()) {
                    try (InputStream in = getAssets().open(asset); OutputStream out = new FileOutputStream(bin)) {
                        byte[] buf = new byte[8192]; int n; while ((n = in.read(buf)) >= 0) out.write(buf, 0, n);
                    }
                    bin.setExecutable(true, false);
                }
                ProcessBuilder pb = new ProcessBuilder(bin.getAbsolutePath());
                pb.redirectErrorStream(true);
                pb.environment().put("OPENWAVE_NO_OPEN", "1"); pb.environment().put("OPENWAVE_PRINT", "1");
                server = pb.start();
                try (BufferedReader r = new BufferedReader(new InputStreamReader(server.getInputStream()))) {
                    String line; while ((line = r.readLine()) != null) {
                        if (line.startsWith("http://")) { final String url = line.trim(); runOnUiThread(() -> webView.loadUrl(url)); break; }
                    }
                }
            } catch (Exception e) { runOnUiThread(() -> webView.loadData("<meta name='viewport' content='width=device-width'><body style='background:#101111;color:#f4f5ef;font:16px sans-serif;padding:32px'><h2>OpenWave не запустился</h2><p>Архитектура: " + Build.SUPPORTED_ABIS[0] + "</p><pre style='white-space:pre-wrap;color:#f99'>" + e + "</pre></body>", "text/html", "UTF-8")); }
        });
    }
    @Override public void onBackPressed() { if (webView != null && webView.canGoBack()) webView.goBack(); else super.onBackPressed(); }
    @Override protected void onDestroy() { if (server != null) server.destroy(); if (webView != null) webView.destroy(); super.onDestroy(); }
}

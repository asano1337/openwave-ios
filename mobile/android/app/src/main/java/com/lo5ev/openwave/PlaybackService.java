package com.lo5ev.openwave;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;

/**
 * Keeps the WebView process alive while the HTML audio element is playing.
 * The actual audio remains controlled by the OpenWave WebView/media session.
 */
public final class PlaybackService extends Service {
    private static final String CHANNEL_ID = "openwave_playback";
    private static final int NOTIFICATION_ID = 801;

    @Override public void onCreate() {
        super.onCreate();
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel channel = new NotificationChannel(
                    CHANNEL_ID,
                    "OpenWave playback",
                    NotificationManager.IMPORTANCE_LOW
            );
            channel.setDescription("Фоновое воспроизведение OpenWave");
            manager.createNotificationChannel(channel);
        }
    }

    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        Intent open = new Intent(this, MainActivity.class);
        open.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        PendingIntent pending = PendingIntent.getActivity(
                this,
                0,
                open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );
        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(this, CHANNEL_ID)
                : new Notification.Builder(this);
        Notification notification = builder
                .setSmallIcon(android.R.drawable.ic_media_play)
                .setContentTitle("OpenWave")
                .setContentText("Музыка играет в фоне")
                .setContentIntent(pending)
                .setOngoing(true)
                .setCategory(Notification.CATEGORY_TRANSPORT)
                .build();
        startForeground(NOTIFICATION_ID, notification);
        return START_STICKY;
    }

    @Override public void onDestroy() {
        stopForeground(STOP_FOREGROUND_REMOVE);
        super.onDestroy();
    }

    @Override public IBinder onBind(Intent intent) {
        return null;
    }
}
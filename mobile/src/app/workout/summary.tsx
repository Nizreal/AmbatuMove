import { useState } from "react";
import { router } from "expo-router";
import { Button, Notice, Screen } from "../../components/ui";
import { SessionSummary } from "../../components/session-summary";
import { useStore } from "../../hooks/store";
export default function Summary() {
  const s = useStore(),
    [session] = useState(s.active),
    [busy, setBusy] = useState(false),
    [saved, setSaved] = useState(false),
    [error, setError] = useState("");
  async function save() {
    if (!session || busy || saved) return;
    setBusy(true);
    setError("");
    try {
      await s.save(session);
      setSaved(true);
    } catch {
      setError("Hasil belum tersimpan. Coba lagi; hasil sesi tetap tersedia.");
    } finally {
      setBusy(false);
    }
  }
  if (!session?.finishedAt)
    return (
      <Screen>
        <Notice message="Belum ada hasil sesi untuk disimpan." />
        <Button title="Beranda" onPress={() => router.replace("/")} />
      </Screen>
    );
  return (
    <Screen>
      <SessionSummary session={session} />
      {!!error && <Notice error message={error} />}
      <Notice
        message={
          saved
            ? "Hasil tersimpan di perangkat."
            : "Simpan hasil untuk menambahkannya ke riwayat."
        }
      />
      {!saved ? (
        <Button
          title="Simpan hasil latihan"
          busy={busy}
          onPress={() => void save()}
        />
      ) : (
        <>
          <Button
            title="Lihat riwayat"
            onPress={() => router.replace("/(tabs)/history")}
          />
          <Button
            secondary
            title="Kembali ke beranda"
            onPress={() => router.replace("/")}
          />
        </>
      )}
    </Screen>
  );
}

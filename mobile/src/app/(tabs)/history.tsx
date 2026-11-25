import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { useMemo, useState } from "react";
import { Pressable, View } from "react-native";
import { Button, Chips, Notice, Screen, Txt, styles } from "../../components/ui";
import { sessionCalories } from "../../domain/workout";
import { useStore } from "../../hooks/store";
import { WorkoutLogRecord } from "../../services/ambatumove";
import { colors } from "../../theme";

type Period = "all" | "week" | "month";
function isInPeriod(log: WorkoutLogRecord, period: Period) {
  if (period === "all") return true;
  const elapsed = Date.now() - Date.parse(log.finished_at || log.started_at);
  return elapsed >= 0 && elapsed <= (period === "week" ? 7 : 31) * 24 * 60 * 60 * 1000;
}
const dateLabel = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Waktu tidak tersedia" : date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
};

export default function History() {
  const s = useStore();
  const [period, setPeriod] = useState<Period>("all");
  const logs = useMemo(() => s.remoteHistory.filter((log) => isInPeriod(log, period)), [s.remoteHistory, period]);
  return (
    <Screen>
      <Txt variant="title">Riwayat workout</Txt>
      <Txt color={colors.muted}>Sesi yang disimpan pada akun AmbatuMove.</Txt>
      {s.historyLoading && <Notice message="Memuat riwayat akun…" />}
      {s.historyError && <Notice error message={`Riwayat belum dapat dimuat. ${s.historyError}`} />}
      {s.historyError && <Button secondary title="Coba lagi" onPress={() => void s.refreshHistory()} />}
      {s.remoteHistory.length > 0 && <Chips values={["all", "week", "month"] as const} selected={period} names={{ all: "Semua", week: "7 hari", month: "30 hari" }} onSelect={setPeriod} />}
      {!s.historyLoading && !s.historyError && s.remoteHistory.length === 0 && <View style={styles.emptyState}><MaterialCommunityIcons name="calendar-blank-outline" size={32} color={colors.primary} /><Txt variant="heading">Belum ada sesi tersimpan</Txt><Txt color={colors.muted}>Selesaikan workout dan simpan hasil untuk melihatnya di sini.</Txt><Button title="Rencanakan latihan" onPress={() => router.push("/recommendation/form")} /></View>}
      {!s.historyLoading && logs.length === 0 && s.remoteHistory.length > 0 && <Notice message="Tidak ada sesi pada periode ini. Pilih rentang waktu lain." />}
      {logs.map((log) => {
        const local = s.history.find((session) => session.startedAt === log.started_at);
        const title = log.notes?.trim() || local?.plan.title || "Sesi latihan";
        const status = log.status === "completed" ? "Selesai" : log.status === "skipped" ? "Dilewati" : log.status === "partial" ? "Parsial" : log.status;
        return <Pressable key={log.id} accessibilityRole="button" onPress={() => router.push({ pathname: "/history/[id]", params: { id: log.id } })} style={({ pressed }) => [styles.historyItem, pressed && { backgroundColor: colors.primarySoft }]}>
          <View style={styles.historyIcon}><MaterialCommunityIcons name="dumbbell" size={20} color={colors.primary} /></View>
          <View style={{ flex: 1, gap: 3 }}><Txt variant="label">{title}</Txt><Txt variant="small" color={colors.muted}>{dateLabel(log.started_at)}</Txt><Txt variant="small">{log.items.length} gerakan · {status}</Txt>{local ? <Txt variant="small" color={colors.primary}>≈ {sessionCalories(local).totalKcal} kcal · perkiraan lokal</Txt> : null}</View>
          <MaterialCommunityIcons name="chevron-right" size={24} color={colors.primary} />
        </Pressable>;
      })}
      {s.remoteHistory.length > 0 && <Notice message="Catatan workout dimuat dari server. Kalori hanya ditampilkan bila rincian lokal sesi tersedia; nilainya tetap perkiraan." />}
    </Screen>
  );
}

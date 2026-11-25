import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { useEffect, useRef, useState } from "react";
import { Pressable, View } from "react-native";
import { Button, Notice, Progress as ProgressBar, Screen, Txt, styles } from "../../components/ui";
import { useStore } from "../../hooks/store";
import { WorkoutLogRecord, describeApiError, getProgression } from "../../services/ambatumove";
import { featureStorage } from "../../services/user-features";
import { colors, space } from "../../theme";

const dayKey = (value: string) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
};
const completed = (log: WorkoutLogRecord) => log.status.toLowerCase() === "completed";
const localKey = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
function streak(logs: WorkoutLogRecord[], now: number | null) {
  if (now === null) return 0;
  const days = new Set(logs.filter(completed).map((log) => dayKey(log.finished_at || log.started_at)).filter(Boolean));
  const cursor = new Date(now);
  if (!days.has(localKey(cursor))) cursor.setDate(cursor.getDate() - 1);
  let count = 0;
  while (days.has(localKey(cursor))) {
    count += 1;
    cursor.setDate(cursor.getDate() - 1);
  }
  return count;
}

function Link({ title, detail, onPress }: { title: string; detail: string; onPress: () => void }) {
  return <Pressable accessibilityRole="button" onPress={onPress} style={styles.dashboardAction}><View style={{ flex: 1, gap: 3 }}><Txt variant="label">{title}</Txt><Txt variant="small" color={colors.muted}>{detail}</Txt></View><MaterialCommunityIcons name="chevron-right" size={22} color={colors.muted} /></Pressable>;
}

export default function Progress() {
  const { user, remoteHistory, historyLoading, historyError, withAuth } = useStore();
  const [weeklyGoal, setWeeklyGoal] = useState(3);
  const [suggestions, setSuggestions] = useState<Record<string, unknown>[]>([]);
  const [suggestionError, setSuggestionError] = useState("");
  const [loadedForUser, setLoadedForUser] = useState<string | null>(null);
  const suggestionLoading = !!user && loadedForUser !== user;
  const [now, setNow] = useState<number | null>(null);
  const withAuthRef = useRef(withAuth);
  useEffect(() => { withAuthRef.current = withAuth; }, [withAuth]);
  useEffect(() => { if (user) void featureStorage.weeklyGoal(user).then(setWeeklyGoal); }, [user]);
  useEffect(() => { const id = setTimeout(() => setNow(Date.now()), 0); return () => clearTimeout(id); }, []);
  useEffect(() => {
    if (!user) return;
    let alive = true;
    void withAuthRef.current((token) => getProgression(token, Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"))
      .then((data) => { if (alive) { setSuggestions(Array.isArray(data.items) ? data.items : []); setSuggestionError(""); } })
      .catch((error) => { if (alive) setSuggestionError(describeApiError(error)); })
      .finally(() => { if (alive) setLoadedForUser(user); });
    return () => { alive = false; };
  }, [user]);
  const weekly = now === null ? 0 : remoteHistory.filter((log) => completed(log) && now - Date.parse(log.finished_at || log.started_at) <= 7 * 24 * 60 * 60 * 1000 && now >= Date.parse(log.finished_at || log.started_at)).length;
  const runStreak = streak(remoteHistory, now);

  return (
    <Screen>
      <View style={{ gap: 4 }}><Txt variant="title">Progres latihan</Txt><Txt color={colors.muted}>Ringkasan dari sesi yang tersimpan di akun Anda.</Txt></View>
      {historyLoading && <Notice message="Memuat aktivitas akun…" />}
      {historyError && <Notice error message={`Progres belum tersedia. ${historyError}`} />}
      <View style={styles.planCard}>
        <View style={{ flexDirection: "row", justifyContent: "space-between", gap: space.md }}>
          <View style={{ flex: 1, gap: 2 }}><Txt variant="small" color={colors.muted}>Sesi selesai · 7 hari</Txt><Txt variant="stat">{weekly}</Txt></View>
          <View style={{ flex: 1, gap: 2 }}><Txt variant="small" color={colors.muted}>Hari beruntun</Txt><Txt variant="stat">{runStreak}</Txt></View>
        </View>
        <ProgressBar value={weekly / weeklyGoal} label={`Target mingguan · ${weekly}/${weeklyGoal} sesi`} />
        <Button secondary title="Atur target mingguan" onPress={() => router.push("/weekly-goal")} />
      </View>
      <View style={styles.formSection}>
        <Txt variant="heading">Perjalanan latihan</Txt>
        {!remoteHistory.length && !historyLoading && !historyError ? <Notice message="Belum ada sesi workout yang selesai tersimpan di akun Anda." /> : null}
        <Button title="Buka riwayat workout" onPress={() => router.navigate("/(tabs)/history")} />
        <Txt variant="small" color={colors.muted}>Hari beruntun dihitung dari hari kalender dengan sesi berstatus selesai, berdasarkan data akun.</Txt>
      </View>
      <View style={styles.formSection}>
        <Txt variant="heading">Saran progresi</Txt>
        {suggestionLoading && <Notice message="Memuat data progresi akun…" />}
        {suggestionError && <Notice error message={`Saran belum dapat dimuat. ${suggestionError}`} />}
        {!suggestionLoading && !suggestionError && suggestions.length === 0 && <Notice message="Saran progresi muncul setelah tersedia cukup riwayat pada akun Anda." />}
        {suggestions.map((item, index) => <View key={`${String(item.exercise_id)}-${index}`} style={styles.card}><Txt variant="label">{String(item.exercise_id ?? "Gerakan")}</Txt><Txt>{String(item.explanation ?? item.suggestion ?? "Saran tersedia berdasarkan aktivitas akun.")}</Txt></View>)}
      </View>
      <View style={styles.formSection}>
        <Txt variant="heading">Dukungan kesehatan</Txt>
        <Link title="Kesehatan dan nutrisi" detail="Protein harian dan bantuan profesional" onPress={() => router.push("/wellness")} />
        <Link title="Coach latihan AI" detail="Tanya jawab via layanan AmbatuMove" onPress={() => router.push("/coach")} />
      </View>
    </Screen>
  );
}

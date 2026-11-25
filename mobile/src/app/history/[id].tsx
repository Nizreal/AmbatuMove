import { useCallback, useEffect, useState } from "react";
import { View } from "react-native";
import { useLocalSearchParams } from "expo-router";
import { Button, Notice, Screen, Txt, styles } from "../../components/ui";
import { SessionSummary } from "../../components/session-summary";
import { useStore } from "../../hooks/store";
import { WorkoutLogRecord, describeApiError, getWorkoutLog } from "../../services/ambatumove";
import { colors } from "../../theme";

export default function Detail() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { withAuth, history } = useStore();
  const [log, setLog] = useState<WorkoutLogRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    if (!id) return;
    try { setLog(await withAuth((token) => getWorkoutLog(token, id))); }
    catch (e) { setError(describeApiError(e)); setLog(null); }
    finally { setLoading(false); }
  }, [id, withAuth]);
  useEffect(() => { void Promise.resolve().then(load); }, [load]);
  const local = log ? history.find((session) => session.startedAt === log.started_at) : undefined;
  return (
    <Screen>
      {loading && <Notice message="Memuat detail sesi dari AmbatuMove…" />}
      {error && <Notice error message={`Detail sesi belum tersedia. ${error}`} />}
      {error && <Button secondary title="Coba lagi" onPress={() => { setLoading(true); setError(""); void load(); }} />}
      {log && <>
        <Txt variant="title">{log.notes?.trim() || local?.plan.title || "Detail workout"}</Txt>
        <View style={styles.planCard}><Txt variant="label">Status</Txt><Txt>{log.status}</Txt><Txt variant="small" color={colors.muted}>Mulai: {new Date(log.started_at).toLocaleString()}</Txt><Txt variant="small" color={colors.muted}>Selesai: {new Date(log.finished_at).toLocaleString()}</Txt></View>
        {local ? <SessionSummary session={local} /> : <View style={styles.formSection}><Txt variant="heading">Gerakan tercatat</Txt>{log.items.length ? log.items.map((item, index) => <Txt key={`${item}-${index}`}>{item}</Txt>) : <Notice message="Rincian gerakan tidak disediakan oleh riwayat server." />}</View>}
        <Notice message="Riwayat diambil dari akun AmbatuMove. Endpoint yang tersedia tidak menyediakan penghapusan catatan workout." />
      </>}
      {!loading && !error && !log && <Notice message="Sesi tidak ditemukan pada akun ini." />}
    </Screen>
  );
}

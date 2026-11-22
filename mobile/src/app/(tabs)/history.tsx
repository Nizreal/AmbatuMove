import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { useState } from "react";
import { router } from "expo-router";
import { Pressable, View } from "react-native";
import {
  Button,
  Chips,
  Notice,
  Screen,
  Txt,
  styles,
} from "../../components/ui";
import { dateLabel } from "../../components/session-summary";
import { Alert } from "../../components/confirm";
import { useStore } from "../../hooks/store";
import { counts, Session, sessionCalories } from "../../domain/workout";
import { colors } from "../../theme";

type Period = "all" | "week" | "month";
function isInPeriod(session: Session, period: Period) {
  if (period === "all") return true;
  const elapsed = Date.now() - Date.parse(session.startedAt);
  return (
    elapsed >= 0 &&
    elapsed <= (period === "week" ? 7 : 31) * 24 * 60 * 60 * 1000
  );
}

export default function History() {
  const s = useStore();
  const [period, setPeriod] = useState<Period>("all");
  const sessions = s.history.filter((session) => isInPeriod(session, period));
  return (
    <Screen>
      <Txt variant="title">Riwayat workout</Txt>
      <Txt color={colors.muted}>
        Sesi dan progres yang tersimpan di perangkat.
      </Txt>
      {!!s.error && <Notice error message={s.error} />}
      {!!s.error && (
        <Button
          secondary
          title="Coba simpan lagi"
          onPress={() => void s.retrySave()}
        />
      )}
      {s.history.length ? (
        <Chips
          values={["all", "week", "month"] as const}
          selected={period}
          names={{ all: "Semua", week: "7 hari", month: "30 hari" }}
          onSelect={setPeriod}
        />
      ) : null}
      {!s.history.length ? (
        <View style={styles.emptyState}>
          <MaterialCommunityIcons
            name="calendar-blank-outline"
            size={34}
            color={colors.primary}
          />
          <Txt variant="heading">Belum ada sesi</Txt>
          <Txt color={colors.muted}>
            Setelah latihan, hasilnya akan tersimpan di sini.
          </Txt>
          <Button
            title="Rencanakan latihan"
            onPress={() => router.push("/recommendation/form")}
          />
        </View>
      ) : !sessions.length ? (
        <Notice message="Tidak ada sesi pada periode ini. Pilih rentang waktu lain untuk melihat riwayat." />
      ) : (
        sessions.map((session) => {
          const stat = counts(session);
          const calories = sessionCalories(session);
          const duration = session.finishedAt
            ? Math.max(
                1,
                Math.round(
                  (Date.parse(session.finishedAt) -
                    Date.parse(session.startedAt)) /
                    60000,
                ),
              )
            : null;
          return (
            <View key={session.id} style={styles.historyItem}>
              <View style={styles.historyIcon}>
                <MaterialCommunityIcons
                  name="dumbbell"
                  size={20}
                  color={colors.primary}
                />
              </View>
              <View style={{ flex: 1, gap: 3 }}>
                <Txt variant="label">{session.plan.title}</Txt>
                <Txt variant="small" color={colors.muted}>
                  {dateLabel(session.startedAt)}
                </Txt>
                <Txt variant="small">
                  {session.plan.items.length} gerakan
                  {duration ? ` · ${duration} menit` : ""} ·{" "}
                  {session.plan.source === "manual" ? "Manual" : "Rekomendasi"}
                </Txt>
                <Txt variant="small" color={colors.primary}>
                  ≈ {calories.totalKcal} kcal
                </Txt>
                <Txt variant="small" color={colors.muted}>
                  Perkiraan lokal
                </Txt>
                <Txt
                  variant="small"
                  color={
                    session.outcome === "completed"
                      ? colors.success
                      : colors.muted
                  }
                >
                  {session.outcome === "completed" ? "Selesai" : "Parsial"} ·{" "}
                  {stat.completed}/{stat.total} set
                </Txt>
              </View>
              <View style={styles.historyActions}>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={`Lihat ${session.plan.title}`}
                  style={styles.historyAction}
                  onPress={() =>
                    router.push({
                      pathname: "/history/[id]",
                      params: { id: session.id },
                    })
                  }
                >
                  <MaterialCommunityIcons
                    name="chevron-right"
                    size={24}
                    color={colors.primary}
                  />
                </Pressable>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={`Hapus ${session.plan.title}`}
                  style={[styles.historyAction, styles.historyDeleteAction]}
                  onPress={() =>
                    Alert.alert(
                      "Hapus riwayat latihan?",
                      "Sesi ini dan rincian kalori perkiraannya akan dihapus dari perangkat.",
                      [
                        { text: "Batal", style: "cancel" },
                        {
                          text: "Hapus",
                          style: "destructive",
                          onPress: () =>
                            void s
                              .removeHistory(session.id)
                              .catch(() =>
                                Alert.alert(
                                  "Belum terhapus",
                                  "Coba lagi sebentar.",
                                  [{ text: "Tutup" }],
                                ),
                              ),
                        },
                      ],
                    )
                  }
                >
                  <MaterialCommunityIcons
                    name="trash-can-outline"
                    size={19}
                    color={colors.error}
                  />
                </Pressable>
              </View>
            </View>
          );
        })
      )}
    </Screen>
  );
}

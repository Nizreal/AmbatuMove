import { View } from "react-native";
import {
  CALORIE_REFERENCE_WEIGHT_KG,
  Session,
  counts,
  sessionCalories,
} from "../domain/workout";
import { Notice, Txt, styles } from "./ui";
import { colors } from "../theme";
export const dateLabel = (date: string) =>
  new Date(date).toLocaleString("id-ID", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
export function SessionSummary({ session: s }: { session: Session }) {
  const stat = counts(s);
  const calories = sessionCalories(s);
  return (
    <>
      <View style={styles.hero}>
        <Txt variant="small">
          {s.outcome === "completed" ? "Sesi lengkap" : "Sesi parsial"}
        </Txt>
        <Txt variant="stat">
          {stat.completed} dari {stat.total}
          {"\n"}set selesai.
        </Txt>
        <Txt>
          {stat.skipped} dilewati / {stat.pending} belum selesai
        </Txt>
      </View>
      <Txt variant="heading">{s.plan.title}</Txt>
      <Txt>
        {s.plan.source === "manual" ? "Disusun manual" : "Dari rekomendasi"} /{" "}
        {dateLabel(s.startedAt)}
      </Txt>
      {s.finishedAt && (
        <Txt variant="small">
          Waktu sesi:{" "}
          {Math.max(
            1,
            Math.round(
              (Date.parse(s.finishedAt) - Date.parse(s.startedAt)) / 60000,
            ),
          )}{" "}
          menit (termasuk jeda)
        </Txt>
      )}
      <View style={styles.caloriePanel}>
        <View style={{ flex: 1, gap: 2 }}>
          <Txt variant="label">Perkiraan kalori terbakar</Txt>
          <Txt variant="small" color={colors.muted}>
            Estimasi MET · set selesai · referensi {CALORIE_REFERENCE_WEIGHT_KG}{" "}
            kg
          </Txt>
        </View>
        <Txt variant="stat">{calories.totalKcal}</Txt>
        <Txt variant="small">kcal</Txt>
      </View>
      <Notice
        message={`Estimasi lokal berbasis MET dengan bobot acuan ${CALORIE_REFERENCE_WEIGHT_KG} kg; bukan nilai dari API AmbatuMove.`}
      />
      <Txt variant="heading">Rencana dan hasil aktual</Txt>
      {s.plan.items.map((p, i) => (
        <View style={styles.divider} key={`${p.exercise.id}-${i}`}>
          <Txt variant="exercise">
            {i + 1}. {p.exercise.name}
          </Txt>
          <Txt color={colors.muted}>
            Rencana: {p.sets} × {p.target}{" "}
            {p.exercise.timed ? "detik" : "repetisi"}
          </Txt>
          <Txt>
            Aktual: {s.actual[i].filter((v) => v === "completed").length} set
            dikonfirmasi
          </Txt>
          <Txt variant="small" color={colors.primary}>
            Perkiraan: ≈ {calories.items[i].kcal} kcal
          </Txt>
          {s.actual[i].map((v, j) => (
            <Txt key={j} variant="small">
              Set {j + 1}:{" "}
              {v === "completed"
                ? `${p.target} ${p.exercise.timed ? "detik" : "repetisi"} selesai`
                : v === "skipped"
                  ? "dilewati"
                  : "belum selesai"}
            </Txt>
          ))}
        </View>
      ))}
    </>
  );
}

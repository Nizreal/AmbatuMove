import { useLocalSearchParams } from "expo-router";
import { useEffect, useState } from "react";
import { Button, Notice, Screen, Txt } from "../../components/ui";
import { ExerciseInfo, ExercisePhoto } from "../../components/exercise";
import { Exercise } from "../../domain/workout";
import { getExerciseDetails } from "../../services/ambatumove";
import { useStore } from "../../hooks/store";

export default function Detail() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const s = useStore();
  const savedExercise =
    s.plan?.items.find((item) => item.exercise.id === id)?.exercise ??
    s.active?.plan.items.find((item) => item.exercise.id === id)?.exercise ??
    s.history
      .flatMap((session) => session.plan.items.map((item) => item.exercise))
      .find((item) => item.id === id);
  const isRemoteExercise = s.catalog.some((item) => item.id === id);
  const [exercise, setExercise] = useState<Exercise | undefined>(
    s.catalog.find((item) => item.id === id) ?? savedExercise,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    if (!id || (!isRemoteExercise && savedExercise)) return;
    void getExerciseDetails(id)
      .then((item) => {
        if (live) setExercise(item);
      })
      .catch((e) => {
        if (live)
          setError(
            e instanceof Error ? e.message : "Detail gerakan belum tersedia.",
          );
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [id, isRemoteExercise, savedExercise]);
  if (!exercise)
    return (
      <Screen>
        <Notice
          error={!!error}
          message={
            error || (loading ? "Memuat gerakan…" : "Gerakan tidak ditemukan.")
          }
        />
        <Button
          secondary
          title="Coba lagi"
          onPress={() => {
            setError("");
            setLoading(true);
            void getExerciseDetails(id ?? "")
              .then(setExercise)
              .catch((e) =>
                setError(
                  e instanceof Error ? e.message : "Gerakan belum tersedia.",
                ),
              )
              .finally(() => setLoading(false));
          }}
        />
      </Screen>
    );
  return (
    <Screen>
      <ExercisePhoto exercise={exercise} large />
      <ExerciseInfo exercise={exercise} />
      <Txt variant="heading">Panduan gerakan</Txt>
      {exercise.description && <Txt>{exercise.description}</Txt>}
      {exercise.instructions.length ? (
        exercise.instructions.map((instruction, i) => (
          <Txt key={i}>
            {i + 1}. {instruction}
          </Txt>
        ))
      ) : (
        <Notice message="Instruksi langkah demi langkah belum disediakan oleh katalog AmbatuMove." />
      )}
      {!!exercise.cautions?.length && (
        <Notice message={`Perhatian: ${exercise.cautions.join(" · ")}`} />
      )}
      <Txt variant="small">
        {exercise.measurement === "duration"
          ? "Target berbasis durasi"
          : "Target berbasis repetisi"}{" "}
        ·{" "}
        {exercise.level === "beginner"
          ? "Pemula"
          : exercise.level === "intermediate"
            ? "Menengah"
            : "Mahir"}
      </Txt>
      <Notice message="Deskripsi dan petunjuk mengikuti data katalog AmbatuMove. Periksa alat yang diperlukan dan hentikan bila terasa nyeri." />
      {!!error && (
        <Notice
          error
          message="Detail terbaru tidak termuat; menampilkan data katalog terakhir yang tersimpan."
        />
      )}
    </Screen>
  );
}

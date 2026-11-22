import { ActivityIndicator, View } from "react-native";
import { useState } from "react";
import { Image } from "expo-image";
import { router } from "expo-router";
import { Exercise, labels, PlanItem } from "../domain/workout";
import { Button, Txt, styles } from "./ui";
import { colors, radius } from "../theme";
export function ExercisePhoto({
  exercise,
  large = false,
  compact = false,
}: {
  exercise: Exercise;
  large?: boolean;
  compact?: boolean;
}) {
  const size = large ? 240 : compact ? 68 : 100;
  const [loadedId, setLoadedId] = useState(""),
    [errorId, setErrorId] = useState("");
  return exercise.image ? (
    <View style={{ height: size, width: compact ? size : "100%" }}>
      <Image
        key={exercise.id}
        source={exercise.image}
        onLoad={() => setLoadedId(exercise.id)}
        onError={() => setErrorId(exercise.id)}
        accessibilityLabel={`Referensi posisi ${exercise.name}`}
        contentFit="contain"
        style={{
          height: size,
          width: compact ? size : "100%",
          backgroundColor: colors.surface,
          borderRadius: radius.md,
        }}
      />
      {errorId === exercise.id ? (
        <View
          style={{ position: "absolute", inset: 0, justifyContent: "center" }}
        >
          <Txt variant="small">
            Foto tidak dapat dimuat. Panduan teks tetap tersedia.
          </Txt>
        </View>
      ) : loadedId !== exercise.id ? (
        <View
          style={{ position: "absolute", inset: 0, justifyContent: "center" }}
        >
          <ActivityIndicator
            accessibilityLabel="Memuat foto gerakan"
            color={colors.primary}
          />
        </View>
      ) : null}
    </View>
  ) : (
    <View
      accessibilityLabel={`Foto referensi belum tersedia untuk ${exercise.name}`}
      style={{
        height: size,
        width: compact ? size : "100%",
        borderRadius: radius.md,
        backgroundColor: colors.accent,
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <Txt variant={compact ? "label" : "heading"} color={colors.primary}>
        {exercise.name.slice(0, 1).toUpperCase()}
      </Txt>
    </View>
  );
}
export function ExerciseInfo({ exercise }: { exercise: Exercise }) {
  return (
    <View style={styles.group}>
      <Txt variant="exercise">{exercise.name}</Txt>
      <Txt variant="small" color={colors.muted}>
        {labels[exercise.muscle]} / {labels[exercise.level]}
      </Txt>
      <Txt variant="small">
        {exercise.equipment.length
          ? exercise.equipment.map((e) => labels[e]).join(", ")
          : "Tanpa alat"}
      </Txt>
    </View>
  );
}
export function PlanRow({
  item,
  index,
  children,
}: {
  item: PlanItem;
  index: number;
  children?: React.ReactNode;
}) {
  return (
    <View style={styles.planCard}>
      <Txt variant="small" color={colors.primary}>
        Gerakan {index + 1}
      </Txt>
      <View style={styles.planRow}>
        <ExercisePhoto exercise={item.exercise} compact />
        <View style={{ flex: 1, gap: 3 }}>
          <Txt variant="exercise">{item.exercise.name}</Txt>
          <Txt variant="small" color={colors.muted}>
            {labels[item.exercise.muscle]} · {labels[item.exercise.level]}
          </Txt>
          <Txt variant="small">
            {item.sets} set × {item.target}{" "}
            {item.exercise.timed ? "detik" : "repetisi"}
          </Txt>
        </View>
      </View>
      <Txt variant="small" color={colors.muted}>
        {item.exercise.equipment.length
          ? item.exercise.equipment.map((e) => labels[e]).join(", ")
          : "Tanpa alat"}{" "}
        · jeda {item.rest} detik
      </Txt>
      <Button
        secondary
        title="Lihat panduan"
        onPress={() =>
          router.push({
            pathname: "/exercise/[id]",
            params: { id: item.exercise.id },
          })
        }
      />
      {children}
    </View>
  );
}

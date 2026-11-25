import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { Pressable, View } from "react-native";
import { Screen, Txt, styles } from "../components/ui";
import { colors, space } from "../theme";

function WellnessAction({
  icon,
  title,
  detail,
  onPress,
}: {
  icon: keyof typeof MaterialCommunityIcons.glyphMap;
  title: string;
  detail: string;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => [
        styles.dashboardAction,
        pressed && { backgroundColor: colors.primarySoft },
      ]}
    >
      <View style={styles.dashboardActionIcon}>
        <MaterialCommunityIcons name={icon} size={21} color={colors.primary} />
      </View>
      <View style={{ flex: 1, gap: 2 }}>
        <Txt variant="label">{title}</Txt>
        <Txt variant="small" color={colors.muted}>{detail}</Txt>
      </View>
      <MaterialCommunityIcons name="chevron-right" size={23} color={colors.muted} />
    </Pressable>
  );
}

export default function Wellness() {
  return (
    <Screen>
      <View style={{ gap: space.xs }}>
        <Txt variant="title">Kesehatan</Txt>
        <Txt color={colors.muted}>
          Pendamping latihan untuk nutrisi harian dan dukungan profesional.
        </Txt>
      </View>
      <View style={{ gap: space.sm }}>
        <WellnessAction
          icon="food-apple-outline"
          title="Protein harian"
          detail="Catat asupan dan target protein · AmbatuNourish"
          onPress={() => router.push("/protein")}
        />
        <WellnessAction
          icon="medical-bag"
          title="Bantuan profesional"
          detail="Ketersediaan integrasi direktori · AmbatuFit"
          onPress={() => router.push("/professionals")}
        />
      </View>
      <Txt variant="small" color={colors.muted}>
        Catatan protein tersimpan di perangkat. Direktori AmbatuFit belum dapat
        diakses; profil atau jadwal tidak akan ditampilkan dari data contoh.
      </Txt>
    </Screen>
  );
}

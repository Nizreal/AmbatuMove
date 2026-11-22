import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { ActivityIndicator, View } from "react-native";
import { useFonts } from "expo-font";
import { Provider, useStore } from "../hooks/store";
import { Button, Notice, Screen } from "../components/ui";
import { colors, type } from "../theme";
function Navigation() {
  const s = useStore();
  if (!s.ready)
    return (
      <Screen>
        {s.error ? (
          <>
            <Notice error message={s.error} />
            <Button title="Muat ulang" onPress={() => void s.reload()} />
          </>
        ) : (
          <ActivityIndicator color={colors.primary} />
        )}
      </Screen>
    );
  return (
    <>
      <StatusBar style="dark" />
      <Stack
        screenOptions={{
          headerStyle: { backgroundColor: colors.background },
          headerTintColor: colors.text,
          headerTitleStyle: type.header,
          headerShadowVisible: false,
          contentStyle: { backgroundColor: colors.background },
        }}
      >
        <Stack.Protected guard={!s.user}>
          <Stack.Screen name="login" options={{ headerShown: false }} />
        </Stack.Protected>
        <Stack.Protected guard={!!s.user}>
          <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
          <Stack.Screen
            name="recommendation/form"
            options={{ title: "Kondisi latihan" }}
          />
          <Stack.Screen
            name="recommendation/result"
            options={{ title: "Rencana untuk Anda" }}
          />
          <Stack.Screen
            name="manual/builder"
            options={{ title: "Susun latihan" }}
          />
          <Stack.Screen
            name="catalog"
            options={{ title: "Katalog gerakan", presentation: "modal" }}
          />
          <Stack.Screen
            name="exercise/[id]"
            options={{ title: "Panduan gerakan" }}
          />
          <Stack.Screen
            name="workout/active"
            options={{ title: "Latihan berlangsung", gestureEnabled: false }}
          />
          <Stack.Screen
            name="workout/summary"
            options={{ title: "Hasil latihan", headerBackVisible: false }}
          />
          <Stack.Screen
            name="history/[id]"
            options={{ title: "Detail latihan" }}
          />
          <Stack.Screen
            name="protein/index"
            options={{ title: "Protein harian" }}
          />
          <Stack.Screen
            name="protein/scan"
            options={{ title: "Pindai barcode" }}
          />
          <Stack.Screen
            name="professionals"
            options={{ title: "Bantuan profesional" }}
          />
        </Stack.Protected>
      </Stack>
    </>
  );
}
export default function Layout() {
  const [fontsLoaded, fontError] = useFonts({
    BarlowSemiCondensed_600SemiBold: require("@expo-google-fonts/barlow-semi-condensed/600SemiBold/BarlowSemiCondensed_600SemiBold.ttf"),
    BarlowSemiCondensed_700Bold: require("@expo-google-fonts/barlow-semi-condensed/700Bold/BarlowSemiCondensed_700Bold.ttf"),
    PlusJakartaSans_400Regular: require("@expo-google-fonts/plus-jakarta-sans/400Regular/PlusJakartaSans_400Regular.ttf"),
    PlusJakartaSans_500Medium: require("@expo-google-fonts/plus-jakarta-sans/500Medium/PlusJakartaSans_500Medium.ttf"),
    PlusJakartaSans_600SemiBold: require("@expo-google-fonts/plus-jakarta-sans/600SemiBold/PlusJakartaSans_600SemiBold.ttf"),
  });
  return (
    <SafeAreaProvider>
      <Provider>
        {fontsLoaded || fontError ? (
          <Navigation />
        ) : (
          <View
            style={{
              flex: 1,
              alignItems: "center",
              justifyContent: "center",
              backgroundColor: colors.background,
            }}
          >
            <ActivityIndicator color={colors.primary} />
          </View>
        )}
      </Provider>
    </SafeAreaProvider>
  );
}

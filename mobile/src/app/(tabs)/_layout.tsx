import { Tabs } from "expo-router";
import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { colors, type } from "../../theme";
export default function Layout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: colors.primary,
        tabBarInactiveTintColor: colors.muted,
        headerStyle: { backgroundColor: colors.background },
        headerTintColor: colors.text,
        headerShadowVisible: false,
        tabBarStyle: { backgroundColor: colors.surface },
        tabBarLabelStyle: type.navigation,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "AmbatuMove",
          tabBarLabel: "Beranda",
          tabBarIcon: ({ color, size }) => (
            <MaterialCommunityIcons
              name="home-outline"
              color={typeof color === "string" ? color : colors.primary}
              size={size}
            />
          ),
        }}
      />
      <Tabs.Screen
        name="history"
        options={{
          title: "Riwayat latihan",
          tabBarLabel: "Riwayat",
          tabBarIcon: ({ color, size }) => (
            <MaterialCommunityIcons
              name="history"
              color={typeof color === "string" ? color : colors.primary}
              size={size}
            />
          ),
        }}
      />
      <Tabs.Screen
        name="wellness"
        options={{
          title: "Kesehatan",
          tabBarLabel: "Kesehatan",
          tabBarIcon: ({ color, size }) => (
            <MaterialCommunityIcons
              name="heart-pulse"
              color={typeof color === "string" ? color : colors.primary}
              size={size}
            />
          ),
        }}
      />
      <Tabs.Screen
        name="profile"
        options={{
          title: "Profil",
          tabBarLabel: "Profil",
          tabBarIcon: ({ color, size }) => (
            <MaterialCommunityIcons
              name="account-outline"
              color={typeof color === "string" ? color : colors.primary}
              size={size}
            />
          ),
        }}
      />
    </Tabs>
  );
}

const verifiedPhotoByExerciseId: Record<string, number> = {
  ex_band_pull_apart: require("../../assets/exercises/Band_Pull_Apart.jpg"),
  ex_bodyweight_squat: require("../../assets/exercises/Bodyweight_Squat.jpg"),
  ex_dead_bug: require("../../assets/exercises/Dead_Bug.jpg"),
  ex_push_up: require("../../assets/figma/push-up.jpeg"),
  ex_lat_pulldown: require("../../assets/figma/lat-pulldown.jpeg"),
};

export const exercisePhoto = (canonicalExerciseId: string) =>
  verifiedPhotoByExerciseId[canonicalExerciseId];

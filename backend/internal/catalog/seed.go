package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type seedExercise struct {
	id, name, category, pattern, difficulty, measurement, space string
	equipment, muscles, secondary, environments, instructions   []string
}

var equipmentSeeds = []Equipment{
	{"bodyweight", "Bodyweight", "bodyweight", true}, {"dumbbell", "Dumbbell", "free_weight", true},
	{"barbell", "Barbell", "free_weight", true}, {"flat_bench", "Flat bench", "bench", true},
	{"adjustable_bench", "Adjustable bench", "bench", true}, {"cable_machine", "Cable machine", "machine", true},
	{"squat_rack", "Squat rack", "rack", true}, {"kettlebell", "Kettlebell", "free_weight", true},
	{"resistance_band", "Resistance band", "band", true}, {"pull_up_bar", "Pull-up bar", "bodyweight", true},
	{"treadmill", "Treadmill", "cardio_machine", true}, {"stationary_bike", "Stationary bike", "cardio_machine", true}, {"bicycle", "Bicycle", "cardio_equipment", true},
	{"rowing_machine", "Rowing machine", "cardio_machine", true}, {"yoga_mat", "Exercise mat", "accessory", true},
	{"medicine_ball", "Medicine ball", "free_weight", true},
}

var muscleSeeds = []struct{ id, label string }{
	{"chest", "Chest"}, {"back", "Back"}, {"shoulders", "Shoulders"}, {"biceps", "Biceps"},
	{"triceps", "Triceps"}, {"forearms", "Forearms"}, {"core", "Core"}, {"quadriceps", "Quadriceps"},
	{"hamstrings", "Hamstrings"}, {"glutes", "Glutes"}, {"calves", "Calves"}, {"full_body", "Full body"},
}

var exerciseSeeds = []seedExercise{
	{"ex_push_up", "Push-Up", "strength", "horizontal_push", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"chest"}, []string{"triceps", "shoulders"}, []string{"home", "gym", "indoor"}, []string{"Set a straight body position.", "Lower your chest with control.", "Press back to the starting position."}},
	{"ex_knee_push_up", "Knee Push-Up", "strength", "horizontal_push", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"chest"}, []string{"triceps"}, []string{"home", "gym", "indoor"}, []string{"Keep a straight line from knees to shoulders.", "Lower with control and press away."}},
	{"ex_diamond_push_up", "Close-Grip Push-Up", "strength", "horizontal_push", "intermediate", "repetitions", "small", []string{"bodyweight"}, []string{"triceps"}, []string{"chest"}, []string{"home", "gym", "indoor"}, []string{"Place hands under the chest.", "Lower under control and press up."}},
	{"ex_pull_up", "Pull-Up", "strength", "vertical_pull", "advanced", "repetitions", "small", []string{"pull_up_bar"}, []string{"back"}, []string{"biceps"}, []string{"home", "gym", "indoor"}, []string{"Hang from a secure bar.", "Pull until the chin clears the bar.", "Lower with control."}},
	{"ex_inverted_row", "Inverted Row", "strength", "horizontal_pull", "intermediate", "repetitions", "medium", []string{"pull_up_bar"}, []string{"back"}, []string{"biceps"}, []string{"gym", "indoor"}, []string{"Keep the body straight under the bar.", "Pull the chest toward the bar and lower."}},
	{"ex_bodyweight_squat", "Bodyweight Squat", "strength", "squat", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"quadriceps"}, []string{"glutes"}, []string{"home", "gym", "indoor"}, []string{"Stand with feet about shoulder width.", "Sit the hips down and back.", "Stand through the whole foot."}},
	{"ex_reverse_lunge", "Reverse Lunge", "strength", "lunge", "beginner", "repetitions", "medium", []string{"bodyweight"}, []string{"quadriceps"}, []string{"glutes"}, []string{"home", "gym", "indoor"}, []string{"Step one foot back.", "Lower with control and return to standing."}},
	{"ex_glute_bridge", "Glute Bridge", "strength", "hip_hinge", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"glutes"}, []string{"hamstrings"}, []string{"home", "gym", "indoor"}, []string{"Lie on your back with knees bent.", "Raise hips until shoulders, hips and knees align."}},
	{"ex_plank", "Front Plank", "strength", "anti_extension", "beginner", "duration", "small", []string{"bodyweight"}, []string{"core"}, nil, []string{"home", "gym", "indoor"}, []string{"Brace the trunk and maintain a neutral body line."}},
	{"ex_side_plank", "Side Plank", "strength", "anti_lateral_flexion", "intermediate", "duration", "small", []string{"bodyweight"}, []string{"core"}, []string{"shoulders"}, []string{"home", "gym", "indoor"}, []string{"Stack shoulders and hips.", "Hold a straight line without rotating."}},
	{"ex_dumbbell_bench_press", "Dumbbell Bench Press", "strength", "horizontal_push", "beginner", "repetitions", "medium", []string{"dumbbell", "flat_bench"}, []string{"chest"}, []string{"triceps", "shoulders"}, []string{"gym", "indoor"}, []string{"Lie on the bench with dumbbells.", "Lower under control.", "Press upward."}},
	{"ex_dumbbell_fly", "Dumbbell Chest Fly", "strength", "horizontal_push", "intermediate", "repetitions", "medium", []string{"dumbbell", "flat_bench"}, []string{"chest"}, []string{"shoulders"}, []string{"gym", "indoor"}, []string{"Use a slight elbow bend.", "Open the arms under control and return."}},
	{"ex_cable_chest_fly", "Cable Chest Fly", "strength", "horizontal_push", "intermediate", "repetitions", "medium", []string{"cable_machine"}, []string{"chest"}, []string{"shoulders"}, []string{"gym", "indoor"}, []string{"Set the handles at chest height.", "Bring the arms together with a soft elbow bend."}},
	{"ex_barbell_bench_press", "Barbell Bench Press", "strength", "horizontal_push", "intermediate", "repetitions", "medium", []string{"barbell", "flat_bench"}, []string{"chest"}, []string{"triceps", "shoulders"}, []string{"gym", "indoor"}, []string{"Set the bar over the chest.", "Lower with control and press evenly."}},
	{"ex_incline_dumbbell_press", "Incline Dumbbell Press", "strength", "horizontal_push", "intermediate", "repetitions", "medium", []string{"dumbbell", "adjustable_bench"}, []string{"chest"}, []string{"shoulders", "triceps"}, []string{"gym", "indoor"}, []string{"Set a comfortable incline.", "Press dumbbells upward without bouncing."}},
	{"ex_dumbbell_row", "Single-Arm Dumbbell Row", "strength", "horizontal_pull", "beginner", "repetitions", "medium", []string{"dumbbell"}, []string{"back"}, []string{"biceps"}, []string{"home", "gym", "indoor"}, []string{"Support the torso.", "Pull the elbow toward the hip and lower slowly."}},
	{"ex_barbell_row", "Barbell Row", "strength", "horizontal_pull", "intermediate", "repetitions", "medium", []string{"barbell"}, []string{"back"}, []string{"biceps", "forearms"}, []string{"gym", "indoor"}, []string{"Hinge at the hips.", "Row the bar toward the lower ribs."}},
	{"ex_cable_row", "Seated Cable Row", "strength", "horizontal_pull", "beginner", "repetitions", "medium", []string{"cable_machine"}, []string{"back"}, []string{"biceps"}, []string{"gym", "indoor"}, []string{"Sit tall and brace.", "Pull the handle toward the torso and return."}},
	{"ex_lat_pulldown", "Lat Pulldown", "strength", "vertical_pull", "beginner", "repetitions", "medium", []string{"cable_machine"}, []string{"back"}, []string{"biceps"}, []string{"gym", "indoor"}, []string{"Grip the bar wider than the shoulders.", "Pull toward the upper chest and return slowly."}},
	{"ex_dumbbell_shoulder_press", "Dumbbell Shoulder Press", "strength", "vertical_push", "beginner", "repetitions", "medium", []string{"dumbbell"}, []string{"shoulders"}, []string{"triceps"}, []string{"home", "gym", "indoor"}, []string{"Brace the trunk.", "Press overhead and lower with control."}},
	{"ex_lateral_raise", "Dumbbell Lateral Raise", "strength", "shoulder_abduction", "beginner", "repetitions", "small", []string{"dumbbell"}, []string{"shoulders"}, nil, []string{"home", "gym", "indoor"}, []string{"Raise arms to a comfortable height.", "Lower slowly without swinging."}},
	{"ex_barbell_back_squat", "Barbell Back Squat", "strength", "squat", "advanced", "repetitions", "medium", []string{"barbell", "squat_rack"}, []string{"quadriceps"}, []string{"glutes", "core"}, []string{"gym", "indoor"}, []string{"Set the bar securely across the upper back.", "Squat to a comfortable depth and stand steadily."}},
	{"ex_goblet_squat", "Goblet Squat", "strength", "squat", "beginner", "repetitions", "medium", []string{"kettlebell"}, []string{"quadriceps"}, []string{"glutes", "core"}, []string{"home", "gym", "indoor"}, []string{"Hold weight close to the chest.", "Sit between the hips and stand."}},
	{"ex_dumbbell_deadlift", "Dumbbell Deadlift", "strength", "hip_hinge", "beginner", "repetitions", "medium", []string{"dumbbell"}, []string{"hamstrings"}, []string{"glutes", "back"}, []string{"home", "gym", "indoor"}, []string{"Hinge at the hips with a neutral spine.", "Stand tall while holding the weights."}},
	{"ex_barbell_deadlift", "Barbell Deadlift", "strength", "hip_hinge", "advanced", "repetitions", "medium", []string{"barbell"}, []string{"hamstrings"}, []string{"glutes", "back", "forearms"}, []string{"gym", "indoor"}, []string{"Brace before lifting.", "Push through the floor and stand tall.", "Lower the bar close to the legs."}},
	{"ex_rdl", "Romanian Deadlift", "strength", "hip_hinge", "intermediate", "repetitions", "medium", []string{"barbell"}, []string{"hamstrings"}, []string{"glutes", "back"}, []string{"gym", "indoor"}, []string{"Keep knees softly bent.", "Hinge until hamstrings feel tension and return."}},
	{"ex_step_up", "Step-Up", "strength", "single_leg", "beginner", "repetitions", "medium", []string{"bodyweight"}, []string{"quadriceps"}, []string{"glutes"}, []string{"home", "gym", "indoor"}, []string{"Use a stable step.", "Step up and down with control."}},
	{"ex_dumbbell_lunge", "Dumbbell Lunge", "strength", "lunge", "intermediate", "repetitions", "medium", []string{"dumbbell"}, []string{"quadriceps"}, []string{"glutes"}, []string{"gym", "indoor"}, []string{"Hold weights at your sides.", "Step and lower under control."}},
	{"ex_calf_raise", "Standing Calf Raise", "strength", "plantar_flexion", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"calves"}, nil, []string{"home", "gym", "indoor"}, []string{"Rise onto the balls of the feet.", "Pause briefly, then lower slowly."}},
	{"ex_biceps_curl", "Dumbbell Biceps Curl", "strength", "elbow_flexion", "beginner", "repetitions", "small", []string{"dumbbell"}, []string{"biceps"}, []string{"forearms"}, []string{"home", "gym", "indoor"}, []string{"Keep elbows near the sides.", "Curl without swinging and lower slowly."}},
	{"ex_cable_triceps_pressdown", "Cable Triceps Pressdown", "strength", "elbow_extension", "beginner", "repetitions", "small", []string{"cable_machine"}, []string{"triceps"}, nil, []string{"gym", "indoor"}, []string{"Keep elbows close to the body.", "Extend the elbows and return under control."}},
	{"ex_band_pull_apart", "Band Pull-Apart", "strength", "horizontal_pull", "beginner", "repetitions", "small", []string{"resistance_band"}, []string{"back"}, []string{"shoulders"}, []string{"home", "gym", "indoor"}, []string{"Hold the band at shoulder height.", "Pull apart while keeping the shoulders relaxed."}},
	{"ex_kettlebell_swing", "Kettlebell Swing", "strength", "hip_hinge", "advanced", "repetitions", "medium", []string{"kettlebell"}, []string{"glutes"}, []string{"hamstrings", "core"}, []string{"gym", "outdoor"}, []string{"Hinge to load the hips.", "Drive the hips forward to swing the bell."}},
	{"ex_mountain_climber", "Mountain Climber", "strength", "core_stability", "intermediate", "duration", "small", []string{"bodyweight"}, []string{"core"}, []string{"shoulders"}, []string{"home", "gym", "indoor"}, []string{"Start in a stable high plank.", "Alternate knee drives at a controlled pace."}},
	{"ex_bird_dog", "Bird Dog", "strength", "anti_rotation", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"core"}, []string{"glutes", "back"}, []string{"home", "gym", "indoor"}, []string{"Begin on hands and knees.", "Extend opposite arm and leg without rotating."}},
	{"ex_dead_bug", "Dead Bug", "strength", "anti_extension", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"core"}, nil, []string{"home", "gym", "indoor"}, []string{"Lie on your back and brace gently.", "Alternate lowering opposite arm and leg."}},
	{"ex_wall_sit", "Wall Sit", "strength", "isometric_squat", "beginner", "duration", "small", []string{"bodyweight"}, []string{"quadriceps"}, []string{"glutes"}, []string{"home", "gym", "indoor"}, []string{"Slide down a wall to a comfortable position.", "Hold with steady breathing."}},
	{"ex_cat_cow", "Cat-Cow Mobility", "mobility", "spinal_mobility", "beginner", "duration", "small", []string{"yoga_mat"}, []string{"back"}, []string{"core"}, []string{"home", "gym", "indoor"}, []string{"Move gently between rounded and extended spine positions."}},
	{"ex_hip_flexor_mobility", "Half-Kneeling Hip Flexor Mobility", "mobility", "hip_mobility", "beginner", "duration", "small", []string{"yoga_mat"}, []string{"glutes"}, []string{"quadriceps"}, []string{"home", "gym", "indoor"}, []string{"Kneel on a soft surface.", "Shift forward slightly while keeping the trunk upright."}},
	{"ex_hamstring_mobility", "Supine Hamstring Mobility", "mobility", "hamstring_mobility", "beginner", "duration", "small", []string{"yoga_mat"}, []string{"hamstrings"}, nil, []string{"home", "gym", "indoor"}, []string{"Raise one leg comfortably and gently extend the knee."}},
	{"ex_thoracic_rotation", "Open-Book Thoracic Rotation", "mobility", "thoracic_mobility", "beginner", "repetitions", "small", []string{"yoga_mat"}, []string{"back"}, []string{"shoulders"}, []string{"home", "gym", "indoor"}, []string{"Lie on your side with knees bent.", "Rotate the upper arm and chest gently open."}},
	{"ex_ankle_rock", "Knee-to-Wall Ankle Rock", "mobility", "ankle_mobility", "beginner", "repetitions", "small", []string{"bodyweight"}, []string{"calves"}, nil, []string{"home", "gym", "indoor"}, []string{"Keep the heel down while moving the knee forward over the foot."}},
	{"ex_shoulder_circles", "Shoulder Circles", "mobility", "shoulder_mobility", "beginner", "duration", "small", []string{"bodyweight"}, []string{"shoulders"}, nil, []string{"home", "gym", "indoor"}, []string{"Make slow, comfortable circles with the arms."}},
	{"act_walk_outdoor", "Outdoor Walking", "cardio", "walking", "beginner", "duration_distance", "unrestricted", nil, []string{"full_body"}, nil, []string{"outdoor"}, []string{"Choose a safe route and a comfortable pace."}},
	{"act_walk_treadmill", "Treadmill Walking", "cardio", "walking", "beginner", "duration_distance", "medium", []string{"treadmill"}, []string{"full_body"}, nil, []string{"gym", "indoor"}, []string{"Set a comfortable walking speed and stable incline."}},
	{"act_run_outdoor", "Outdoor Running", "cardio", "running", "intermediate", "duration_distance", "unrestricted", nil, []string{"full_body"}, nil, []string{"outdoor"}, []string{"Select a safe route and sustainable pace."}},
	{"act_run_treadmill", "Treadmill Running", "cardio", "running", "intermediate", "duration_distance", "medium", []string{"treadmill"}, []string{"full_body"}, nil, []string{"gym", "indoor"}, []string{"Set the belt speed before beginning and use the safety clip."}},
	{"act_cycle_outdoor", "Outdoor Cycling", "cardio", "cycling", "beginner", "duration_distance", "unrestricted", []string{"bicycle"}, []string{"full_body"}, nil, []string{"outdoor"}, []string{"Use a safe route and appropriate protective equipment."}},
	{"act_cycle_stationary", "Stationary Cycling", "cardio", "cycling", "beginner", "duration_distance", "medium", []string{"stationary_bike"}, []string{"full_body"}, nil, []string{"gym", "indoor"}, []string{"Adjust the bike and choose a comfortable resistance."}},
	{"act_marching_in_place", "Marching in Place", "cardio", "bodyweight", "beginner", "duration", "small", nil, []string{"full_body"}, nil, []string{"home", "gym", "indoor"}, []string{"March at a comfortable pace in place, increasing effort gradually."}},
	{"act_strength_training", "Resistance Training Session", "strength", "strength_training", "beginner", "duration", "small", nil, []string{"full_body"}, nil, []string{"home", "gym", "indoor"}, []string{"Choose a manageable resistance and controlled pace."}},
	{"act_bodyweight_training", "Bodyweight Training Session", "strength", "bodyweight", "beginner", "duration", "small", []string{"bodyweight"}, []string{"full_body"}, nil, []string{"home", "gym", "indoor"}, []string{"Use controlled bodyweight movements and adjust effort as needed."}},
	{"act_mobility_flow", "Mobility Flow", "mobility", "mobility", "beginner", "duration", "small", nil, []string{"full_body"}, nil, []string{"home", "gym", "indoor"}, []string{"Move gently within a comfortable range."}},
}

func SeedReferenceData(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range equipmentSeeds {
		if _, err := tx.ExecContext(ctx, `INSERT INTO equipment(id,label,category,active) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING`, item.ID, item.Label, item.Category, boolInt(item.Active)); err != nil {
			return fmt.Errorf("seed equipment %s: %w", item.ID, err)
		}
	}
	for _, item := range muscleSeeds {
		if _, err := tx.ExecContext(ctx, `INSERT INTO muscle_groups(id,label) VALUES(?,?) ON CONFLICT(id) DO NOTHING`, item.id, item.label); err != nil {
			return fmt.Errorf("seed muscle group %s: %w", item.id, err)
		}
	}
	for _, item := range exerciseSeeds {
		instructions, _ := json.Marshal(item.instructions)
		result, err := tx.ExecContext(ctx, `INSERT INTO exercises(id,name,description,category,movement_pattern,difficulty,measurement_type,min_space,instructions_json,cautions_json,active,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,1,datetime('now')) ON CONFLICT(id) DO NOTHING`, item.id, item.name, "General exercise reference for planning; adjust movement to personal capacity.", item.category, item.pattern, item.difficulty, item.measurement, item.space, string(instructions), "[]")
		if err != nil {
			return fmt.Errorf("seed exercise %s: %w", item.id, err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check seeded exercise %s: %w", item.id, err)
		}
		if inserted == 0 {
			continue
		}
		for _, id := range item.equipment {
			if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_equipment(exercise_id,equipment_id,required) VALUES(?,?,1) ON CONFLICT DO NOTHING`, item.id, id); err != nil {
				return fmt.Errorf("seed equipment requirement for %s: %w", item.id, err)
			}
		}
		for index, id := range append(append([]string{}, item.muscles...), item.secondary...) {
			primary := index < len(item.muscles)
			if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_muscles(exercise_id,muscle_group_id,is_primary) VALUES(?,?,?) ON CONFLICT DO NOTHING`, item.id, id, boolInt(primary)); err != nil {
				return fmt.Errorf("seed muscle for %s: %w", item.id, err)
			}
		}
		for _, environment := range item.environments {
			if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_environments(exercise_id,environment) VALUES(?,?) ON CONFLICT DO NOTHING`, item.id, environment); err != nil {
				return fmt.Errorf("seed environment for %s: %w", item.id, err)
			}
		}
	}
	return tx.Commit()
}

func SeedCount() int { return len(exerciseSeeds) }

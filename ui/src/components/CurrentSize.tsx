import { Box, LinearProgress, Typography } from "@mui/material";

const CurrentSize = (props: {
    status: ClusterStatus;
    prediction: ClusterStatus | null;
    config: Config | null;
}) => {
    const { status, prediction, config } = props;

    // Using config?.max_instances directly or defaulting to 5 if config is undefined/null
    const maxInstances = config ? config.max_instances : 5;

    return (
        <>
            <Box
                sx={{
                    display: "flex",
                    alignItems: "flex-start",
                    flexDirection: "column",
                    width: "calc(50%-10px)",
                }}
            >
                <Typography variant={"h6"}>Size</Typography>
                <Typography variant="h2" component="div">
                    {status.current_active_readers} : {status.optimal_size}
                </Typography>
            </Box>
            {prediction && (
                <Box
                    sx={{
                        display: "flex",
                        alignItems: "flex-end",
                        width: "calc(50%-10px)",
                    }}
                >
                    <Typography variant="h4" component="div" color={"primary"}>
                        {prediction?.current_active_readers} : {prediction.optimal_size}
                    </Typography>
                </Box>
            )}
            <LinearProgress
                sx={{ width: "100%", alignItems: "flex-end" }}
                variant="buffer"
                // Use maxInstances directly with a default value of 5 if config is undefined/null
                value={(status.current_active_readers || 0) / maxInstances * 100}
                valueBuffer={
                    (prediction?.current_active_readers || 0) / maxInstances * 100
                }
            />
        </>
    );
};

export default CurrentSize;

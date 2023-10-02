import React from "react";
import { Backdrop, Box, CircularProgress, Theme, Typography } from "@mui/material";

const Loading = () => {
    return (
        <Backdrop
            sx={{
                color: "#fff",
                zIndex: (theme: Theme) => theme.zIndex.drawer + 1,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
            }}
            open={true}
        >
            <CircularProgress color="inherit" />
            <Box sx={{
                marginTop: "10px",
                animation: "pulse 2s infinite",
                "@keyframes pulse": {
                    "0%": { opacity: 0.5 },
                    "50%": { opacity: 1 },
                    "100%": { opacity: 0.5 },
                },
            }}>
                <Typography>Connecting to service...</Typography>
            </Box>
        </Backdrop>
    );
};

export default Loading;

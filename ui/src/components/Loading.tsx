import { Backdrop, Box, CircularProgress, Theme, Typography } from "@mui/material";
import { ReadyState } from 'react-use-websocket';

interface LoadingProps {
    readyState: ReadyState;
}

const Loading: React.FC<LoadingProps> = ({ readyState }) => {
    const getMessageForReadyState = (readyState: ReadyState): string => {
        switch (readyState) {
            case ReadyState.CONNECTING:
                return "Connecting to service...";
            case ReadyState.OPEN:
                return "WebSocket connection is open.";
            case ReadyState.CLOSING:
                return "WebSocket connection is closing. Reconnecting...";
            case ReadyState.CLOSED:
                return "WebSocket connection is closed. Reconnecting...";
            default:
                return "WebSocket connection is in an unknown state.";
        }
    };

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
            <Box
                sx={{
                    marginTop: "10px",
                    animation: "pulse 2s infinite",
                    "@keyframes pulse": {
                        "0%": { opacity: 0.5 },
                        "50%": { opacity: 1 },
                        "100%": { opacity: 0.5 },
                    },
                }}
            >
                <Typography>{getMessageForReadyState(readyState)}</Typography>
            </Box>
        </Backdrop>
    );
};

export default Loading;
``

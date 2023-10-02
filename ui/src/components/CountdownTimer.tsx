import React, { useState, useEffect } from 'react';
import { Typography, CircularProgress } from '@mui/material';

interface CountdownTimerProps {
    cooldownTimeout: number;
    label: string;
}

const CountdownTimer: React.FC<CountdownTimerProps> = ({ cooldownTimeout, label }) => {
    const [remainingTime, setRemainingTime] = useState(0);

    useEffect(() => {
        const interval = setInterval(() => {
            const now = new Date().getTime();
            const remaining = cooldownTimeout - now;

            if (remaining <= 0) {
                clearInterval(interval);
                setRemainingTime(0);
            } else {
                setRemainingTime(remaining);
            }
        }, 1000);

        return () => clearInterval(interval);
    }, [cooldownTimeout]);

    const seconds = Math.floor(remainingTime / 1000);

    return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }}>
            <Typography variant="subtitle1" gutterBottom>
                {label}
            </Typography>
            <div style={{ position: 'relative' }}>
                <CircularProgress variant="determinate" value={(seconds / 60) * 100} />
                <Typography
                    variant="caption"
                    component="div"
                    style={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                    }}
                >
                    {seconds}
                </Typography>
            </div>
        </div>
    );
};

export default CountdownTimer;
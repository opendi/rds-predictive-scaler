import React from 'react';
import { CircularProgressProps, Icon } from '@mui/material';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';

interface AnimatedIconProps extends CircularProgressProps {
    isScalingIn: boolean;
    isScalingOut: boolean;
}

const AnimatedIcon: React.FC<AnimatedIconProps> = ({ isScalingIn, isScalingOut, ...props }) => {
    return (
        <>
            {(isScalingIn || isScalingOut) && (
                <div style={{ position: 'relative' }}>
                    {isScalingIn && (
                        <Icon
                            component={ArrowDownwardIcon as any}
                            {...props}
                            style={{
                                position: 'absolute',
                                zIndex: 2,
                                animation: 'pulse-in 2s infinite',
                            }}
                        />
                    )}
                    {isScalingOut && (
                        <Icon
                            component={ArrowUpwardIcon as any}
                            {...props}
                            style={{
                                position: 'absolute',
                                zIndex: 2,
                                animation: 'pulse-out 2s infinite',
                            }}
                        />
                    )}
                </div>
            )}
            <style>
                {`
          @keyframes pulse-in {
            0% {
              transform: scale(1);
              opacity: 1;
            }
            50% {
              transform: scale(1.2);
              opacity: 0.7;
            }
            100% {
              transform: scale(1);
              opacity: 1;
            }
          }

          @keyframes pulse-out {
            0% {
              transform: scale(1);
              opacity: 1;
            }
            50% {
              transform: scale(0.8);
              opacity: 0.7;
            }
            100% {
              transform: scale(1);
              opacity: 1;
            }
          }
        `}
            </style>
        </>
    );
};

export default AnimatedIcon;

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useMemo, useRef, useState } from 'react';

import { runKeys } from '@/features/runs/api/run-queries';
import { sessionKeys } from '@/features/sessions/api/session-queries';
import {
  applyChatStreamEvent,
  appendUserMessage,
  buildIngestRequest,
  createConversationID,
  createInitialConsoleChatState,
} from '@/features/chat/model/chat-model';
import type { SessionRecord } from '@/lib/api-client';
import { runtimeClient, toErrorMessage } from '@/lib/api-client';

export function useConsoleChat(activeSession?: SessionRecord) {
  const queryClient = useQueryClient();
  const sequenceRef = useRef(Date.now());
  const [conversationID, setConversationID] = useState(activeSession?.conversation_id || createConversationID());
  const [composerText, setComposerText] = useState('');
  const [state, setState] = useState(createInitialConsoleChatState);

  const mutation = useMutation({
    mutationFn: async (text: string) => {
      const request = buildIngestRequest(conversationID, sequenceRef.current, text, activeSession);
      sequenceRef.current += 1;
      return runtimeClient.sendChat(request, {
        onEvent: async (event) => {
          setState((current) => applyChatStreamEvent(current, event));
        },
      });
    },
    onMutate: (text) => {
      setState((current) => appendUserMessage(current, text));
      setComposerText('');
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: sessionKeys.all }),
        queryClient.invalidateQueries({ queryKey: runKeys.all }),
      ]);
    },
    onError: (error) => {
      setState((current) => ({
        ...current,
        statusLabel: '发送失败',
        errorText: toErrorMessage(error),
      }));
    },
  });

  const api = useMemo(
    () => ({
      composerText,
      setComposerText,
      conversationID,
      setConversationID,
      state,
      sending: mutation.isPending,
      send: async () => {
        const text = composerText.trim();
        if (!text) return;
        await mutation.mutateAsync(text);
      },
      startDraftSession: () => {
        const nextID = createConversationID();
        setConversationID(nextID);
        setComposerText('');
        setState(createInitialConsoleChatState());
      },
    }),
    [composerText, conversationID, mutation, state],
  );

  return api;
}
